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

func Run(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	getenv getenvFunc,
) int {
	if len(args) == 1 && args[0] == "--help" {
		printUsage(stdout)
		return 0
	}
	if len(args) < 2 {
		printUsage(stderr)
		return 2
	}

	var err error
	switch {
	case args[0] == "auth" && args[1] == "login":
		err = authLogin(ctx, args[2:], stdout, stderr, getenv)
	case args[0] == "bookmarks" && args[1] == "list":
		err = bookmarksList(ctx, args[2:], stdout, stderr, getenv)
	default:
		printUsage(stderr)
		return 2
	}

	if err != nil {
		if errors.Is(err, errHelpRequested) {
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		var commandLine commandLineError
		if errors.As(err, &commandLine) {
			return 2
		}
		return 1
	}

	return 0
}

func authLogin(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	getenv getenvFunc,
) error {
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
			printAuthLoginUsage(stdout)
			return errHelpRequested
		}
		printAuthLoginUsage(stderr)
		return commandLineError(err.Error())
	}
	if flags.NArg() > 0 {
		printAuthLoginUsage(stderr)
		return commandLineError(fmt.Sprintf("unexpected argument: %s", flags.Arg(0)))
	}
	if clientID == "" {
		return commandLineError(fmt.Sprintf(
			"client ID is required; set %s or pass --client-id",
			clientIDEnv,
		))
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

func waitForCallback(
	ctx context.Context,
	timeout time.Duration,
	results <-chan callbackResult,
	serverErrors <-chan error,
) (callbackResult, error) {
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
		return callbackResult{}, fmt.Errorf(
			"timed out waiting for OAuth callback after %s",
			timeout,
		)
	}
}

func shutdownServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  xapi-usecase auth login [options]")
	_, _ = fmt.Fprintln(w, "  xapi-usecase bookmarks list [options]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Run a command with --help for command-specific options.")
}

func printAuthLoginUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  xapi-usecase auth login [options]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --client-id CLIENT_ID       OAuth2 client ID")
	_, _ = fmt.Fprintln(w, "  --token-file PATH           path to save the OAuth2 token")
	_, _ = fmt.Fprintln(w, "  --port PORT                 local callback port")
	_, _ = fmt.Fprintln(w, "  --timeout DURATION          browser callback timeout")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(
		w,
		"Environment:\n  %s  OAuth2 client ID used when --client-id is omitted\n",
		clientIDEnv,
	)
}

func printBookmarksListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  xapi-usecase bookmarks list [options]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --token-file PATH           OAuth2 token JSON file")
	_, _ = fmt.Fprintln(w, "  --client-id CLIENT_ID       OAuth2 client ID for refresh")
	_, _ = fmt.Fprintln(w, "  --max-results N             results per page, 1 through 100")
	_, _ = fmt.Fprintln(w, "  --pagination-token TOKEN    page token from meta.next_token")
	_, _ = fmt.Fprintln(w, "  --tweet-fields FIELDS       comma-separated tweet.fields")
	_, _ = fmt.Fprintln(w, "  --expansions EXPANSIONS     comma-separated expansions")
	_, _ = fmt.Fprintln(w, "  --user-fields FIELDS        comma-separated user.fields")
	_, _ = fmt.Fprintln(w, "  --media-fields FIELDS       comma-separated media.fields")
	_, _ = fmt.Fprintln(w, "  --poll-fields FIELDS        comma-separated poll.fields")
	_, _ = fmt.Fprintln(w, "  --place-fields FIELDS       comma-separated place.fields")
	_, _ = fmt.Fprintln(w, "  --timeout DURATION          command timeout, defaults to 30s")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(
		w,
		"Environment:\n  %s  OAuth2 client ID used for refresh when --client-id is omitted\n",
		clientIDEnv,
	)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Required scopes: bookmark.read, tweet.read, users.read.")
	_, _ = fmt.Fprintln(w, "offline.access is required for refresh.")
}
