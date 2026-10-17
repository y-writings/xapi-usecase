package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/y-writings/xapi-usecase/internal/output"
	"github.com/y-writings/xapi-usecase/internal/xapi"
)

type userPostsOptions struct {
	Username, StartTime, EndTime, Output, BearerToken string
	ClientID, TokenFile                               string
	Limit                                             int
	ExcludeReplies, ExcludeReposts                    bool
	Timeout                                           time.Duration
}

func userPosts(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	getenv getenvFunc,
) error {
	o := userPostsOptions{
		Limit: 100, Timeout: 30 * time.Second,
		BearerToken: getenv(bearerTokenEnv), ClientID: getenv(clientIDEnv),
	}
	f := flag.NewFlagSet("xapi-usecase user-posts", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.Username, "username", "", "X username")
	f.StringVar(&o.StartTime, "start-time", "", "RFC3339 inclusive start time")
	f.StringVar(&o.EndTime, "end-time", "", "RFC3339 exclusive end time")
	f.IntVar(&o.Limit, "limit", o.Limit, "maximum posts to retrieve")
	f.BoolVar(&o.ExcludeReplies, "exclude-replies", false, "exclude replies")
	f.BoolVar(&o.ExcludeReposts, "exclude-reposts", false, "exclude reposts")
	f.StringVar(&o.Output, "output", "", "JSON output file")
	f.StringVar(&o.BearerToken, "bearer-token", o.BearerToken, "X API bearer token")
	f.StringVar(&o.TokenFile, "token-file", "", "OAuth2 token JSON file")
	f.StringVar(&o.ClientID, "client-id", o.ClientID, "OAuth2 client ID for refresh")
	f.DurationVar(&o.Timeout, "timeout", o.Timeout, "command timeout")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUserPostsUsage(stdout)
			return errHelpRequested
		}
		printUserPostsUsage(stderr)
		return commandLineError(err.Error())
	}
	o.Username = strings.TrimPrefix(strings.TrimSpace(o.Username), "@")
	if f.NArg() > 0 || o.Username == "" || o.StartTime == "" || o.EndTime == "" {
		return commandLineError(
			"--username, --start-time, and --end-time are required; " +
				"positional arguments are not accepted",
		)
	}
	if o.Limit < 1 || o.Limit > 3200 {
		return commandLineError("--limit must be between 1 and 3200")
	}
	start, err := time.Parse(time.RFC3339, o.StartTime)
	if err != nil {
		return commandLineError("--start-time must be RFC3339")
	}
	end, err := time.Parse(time.RFC3339, o.EndTime)
	if err != nil {
		return commandLineError("--end-time must be RFC3339")
	}
	if !start.Before(end) {
		return commandLineError("--start-time must be before --end-time")
	}
	if o.Timeout <= 0 {
		return commandLineError("--timeout must be greater than 0")
	}

	commandCtx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	client, err := newAuthenticatedClient(commandCtx, o.TokenFile, o.ClientID, o.BearerToken)
	if err != nil {
		return err
	}
	out := output.UserPostsOutput{
		Status: "complete", TargetUser: output.TargetUser{Username: o.Username},
		Conditions: output.UserPostsConditions{
			StartTime: o.StartTime, EndTime: o.EndTime, Limit: o.Limit,
			ExcludeReplies: o.ExcludeReplies, ExcludeReposts: o.ExcludeReposts,
		},
		RetrievedAt: timeNow().UTC().Format(time.RFC3339), Posts: []output.UserPost{},
	}

	var user xapi.UserResponse
	err = client.run(func(api *xapi.Client) error {
		var callErr error
		user, callErr = api.UserByUsername(commandCtx, o.Username)
		return callErr
	})
	if err != nil {
		return finishUserPostsFailure(o.Output, stdout, out, err)
	}
	if len(user.Errors) > 0 || user.Data.ID == "" {
		return finishUserPostsAPIError(o.Output, stdout, out, len(user.Errors))
	}
	out.TargetUser = output.TargetUser{
		ID: user.Data.ID, Name: user.Data.Name, Username: user.Data.Username,
	}

	next := ""
	for len(out.Posts) < o.Limit {
		remaining := o.Limit - len(out.Posts)
		pageSize := min(max(remaining, 5), 100)
		var page xapi.UserPostsResponse
		err = client.run(func(api *xapi.Client) error {
			var callErr error
			page, callErr = api.UserPosts(commandCtx, user.Data.ID, xapi.UserPostsOptions{
				StartTime: o.StartTime, EndTime: o.EndTime, PaginationToken: next,
				MaxResults: pageSize, ExcludeReplies: o.ExcludeReplies,
				ExcludeReposts: o.ExcludeReposts,
			})
			return callErr
		})
		if err != nil {
			return finishUserPostsFailure(o.Output, stdout, out, err)
		}
		for _, post := range page.Data {
			if len(out.Posts) >= o.Limit {
				break
			}
			text := post.Text
			if post.NoteTweet != nil && post.NoteTweet.Text != "" {
				text = post.NoteTweet.Text
			}
			out.Posts = append(out.Posts, output.UserPost{
				ID: post.ID, URL: "https://x.com/" + user.Data.Username + "/status/" + post.ID,
				Text: text, CreatedAt: post.CreatedAt,
			})
		}
		if len(page.Errors) > 0 {
			return finishUserPostsAPIError(o.Output, stdout, out, len(page.Errors))
		}
		next = page.Meta.NextToken
		if len(out.Posts) == o.Limit && (next != "" || len(page.Data) > remaining) {
			out.Status = "limit_reached"
			break
		}
		if next == "" {
			break
		}
	}
	return writeUserPostsOutput(o.Output, stdout, out)
}

func finishUserPostsFailure(path string, w io.Writer, out output.UserPostsOutput, err error) error {
	out.Status = "incomplete"
	out.IncompleteReason = classifySearchError(err)
	if writeErr := writeUserPostsOutput(path, w, out); writeErr != nil {
		return writeErr
	}
	return fmt.Errorf("user-posts incomplete: %s: %w", out.IncompleteReason, err)
}

func finishUserPostsAPIError(
	path string,
	w io.Writer,
	out output.UserPostsOutput,
	count int,
) error {
	out.Status = "incomplete"
	out.IncompleteReason = "api_error"
	if err := writeUserPostsOutput(path, w, out); err != nil {
		return err
	}
	return fmt.Errorf("user-posts incomplete: api_error: response contained %d API errors", count)
}

func writeUserPostsOutput(path string, w io.Writer, out output.UserPostsOutput) error {
	out.PostCount = len(out.Posts)
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path != "" {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return err
		}
		_, err = fmt.Fprintf(
			w, "Saved %d posts to %s (status: %s)\n", out.PostCount, path, out.Status,
		)
		return err
	}
	_, err = w.Write(data)
	return err
}

func printUserPostsUsage(w io.Writer) {
	lines := []string{
		"Usage:",
		"  xapi-usecase user-posts --username USERNAME " +
			"--start-time RFC3339 --end-time RFC3339 [options]",
		"", "Options:",
		"  --username USERNAME      X username, with or without @ (required)",
		"  --start-time RFC3339    inclusive oldest post time (required)",
		"  --end-time RFC3339      exclusive newest post time (required)",
		"  --limit N               maximum posts (1-3200; default 100)",
		"  --exclude-replies       exclude replies", "  --exclude-reposts       exclude reposts",
		"  --output PATH           save JSON to file",
		"  --bearer-token TOKEN    X API bearer token",
		"  --token-file PATH       OAuth2 token JSON file (defaults to the saved login)",
		"  --client-id CLIENT_ID   OAuth2 client ID for refresh",
		"  --timeout DURATION      command timeout",
		"", "Authentication:",
		"  Uses the token saved by auth login unless a bearer token is provided.",
		"  Required scopes: tweet.read, users.read; offline.access for refresh.",
	}
	_, _ = fmt.Fprintln(w, strings.Join(lines, "\n"))
}
