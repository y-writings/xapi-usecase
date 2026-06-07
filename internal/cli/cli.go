package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/y-writings/xapi-usecase/internal/tokenstore"
	"github.com/y-writings/xapi-usecase/internal/xoauth"
)

const (
	clientIDEnv         = "XAPI_USECASE_CLIENT_ID"
	defaultCallbackIP   = "127.0.0.1"
	defaultCallbackPort = 8765
	defaultTimeout      = 5 * time.Minute
	exchangeTimeout     = 30 * time.Second
)

type getenvFunc func(string) string

type commandLineError string

func (e commandLineError) Error() string {
	return string(e)
}

var errHelpRequested = errors.New("help requested")

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, getenv getenvFunc) int {
	if len(args) < 2 || args[0] != "auth" || args[1] != "login" {
		printUsage(stderr)
		return 2
	}

	if err := authLogin(ctx, args[2:], stdout, stderr, getenv); err != nil {
		if errors.Is(err, errHelpRequested) {
			return 0
		}
		fmt.Fprintf(stderr, "Error: %v\n", err)
		var commandLine commandLineError
		if errors.As(err, &commandLine) {
			return 2
		}
		return 1
	}

	return 0
}

func authLogin(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, getenv getenvFunc) error {
	clientID := getenv(clientIDEnv)
	tokenFile := ""
	port := defaultCallbackPort
	timeout := defaultTimeout

	flags := flag.NewFlagSet("xapi-usecase auth login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&clientID, "client-id", clientID, "OAuth2 client ID")
	flags.StringVar(&tokenFile, "token-file", tokenFile, "path to save the OAuth2 token")
	flags.IntVar(&port, "port", port, "local callback port")
	flags.DurationVar(&timeout, "timeout", timeout, "maximum time to wait for the browser callback")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(stdout)
			return errHelpRequested
		}
		printUsage(stderr)
		return commandLineError(err.Error())
	}
	if flags.NArg() > 0 {
		printUsage(stderr)
		return commandLineError(fmt.Sprintf("unexpected argument: %s", flags.Arg(0)))
	}
	if clientID == "" {
		return commandLineError(fmt.Sprintf("client ID is required; set %s or pass --client-id", clientIDEnv))
	}
	if port < 1 || port > 65535 {
		return commandLineError("--port must be between 1 and 65535")
	}
	if timeout <= 0 {
		return commandLineError("--timeout must be greater than 0")
	}
	if tokenFile == "" {
		path, err := tokenstore.DefaultPath()
		if err != nil {
			return err
		}
		tokenFile = path
	}

	codeVerifier, err := xoauth.GenerateCodeVerifier()
	if err != nil {
		return err
	}
	state, err := xoauth.GenerateState()
	if err != nil {
		return err
	}
	codeChallenge := xoauth.CodeChallengeS256(codeVerifier)

	callbackURL := fmt.Sprintf("http://%s:%d/callback", defaultCallbackIP, port)
	client := xoauth.NewClient(clientID)
	authURL, err := client.AuthURL(callbackURL, state, codeChallenge)
	if err != nil {
		return err
	}

	results := make(chan callbackResult, 1)
	server := &http.Server{Handler: newCallbackHandler(state, results)}
	listenerAddr := net.JoinHostPort(defaultCallbackIP, strconv.Itoa(port))
	listener, err := net.Listen("tcp", listenerAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", listenerAddr, err)
	}
	defer shutdownServer(server)

	serverErrors := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	if _, err := fmt.Fprintln(stdout, "Open this URL in your browser:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, authURL); err != nil {
		return err
	}

	result, err := waitForCallback(ctx, timeout, results, serverErrors)
	if err != nil {
		return err
	}
	if result.Err != nil {
		return result.Err
	}

	exchangeCtx, cancel := context.WithTimeout(ctx, exchangeTimeout)
	defer cancel()
	token, err := client.ExchangeCode(exchangeCtx, result.Code, callbackURL, codeVerifier)
	if err != nil {
		return err
	}

	if err := tokenstore.Save(tokenFile, token); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "Token saved to %s\n", tokenFile); err != nil {
		return err
	}

	return nil
}

func waitForCallback(ctx context.Context, timeout time.Duration, results <-chan callbackResult, serverErrors <-chan error) (callbackResult, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-results:
		return result, nil
	case err := <-serverErrors:
		return callbackResult{}, err
	case <-ctx.Done():
		return callbackResult{}, ctx.Err()
	case <-timer.C:
		return callbackResult{}, fmt.Errorf("timed out waiting for OAuth callback after %s", timeout)
	}
}

func shutdownServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func printUsage(stderr io.Writer) {
	fmt.Fprintln(stderr, "Usage:")
	fmt.Fprintln(stderr, "  xapi-usecase auth login [--client-id CLIENT_ID] [--token-file PATH] [--port PORT] [--timeout DURATION]")
	fmt.Fprintln(stderr)
	fmt.Fprintf(stderr, "Environment:\n  %s  OAuth2 client ID used when --client-id is omitted\n", clientIDEnv)
}
