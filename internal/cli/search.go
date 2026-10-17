package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/y-writings/xapi-usecase/internal/output"
	"github.com/y-writings/xapi-usecase/internal/xapi"
)

const bearerTokenEnv = "XAPI_USECASE_BEARER_TOKEN"

type searchOptions struct {
	Query, Language, StartTime, EndTime, Output, BearerToken string
	ClientID, TokenFile                                      string
	Limit                                                    int
	Timeout                                                  time.Duration
}

func search(ctx context.Context, args []string, stdout, stderr io.Writer, getenv getenvFunc) error {
	o := searchOptions{
		Limit: 100, Timeout: 30 * time.Second,
		BearerToken: getenv(bearerTokenEnv), ClientID: getenv(clientIDEnv),
	}
	f := flag.NewFlagSet("xapi-usecase search", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.Query, "query", "", "X search query")
	f.StringVar(&o.Language, "lang", "", "language code")
	f.StringVar(&o.StartTime, "start-time", "", "RFC3339 start time")
	f.StringVar(&o.EndTime, "end-time", "", "RFC3339 end time")
	f.IntVar(&o.Limit, "limit", o.Limit, "maximum posts to retrieve")
	f.StringVar(&o.Output, "output", "", "JSON output file")
	f.StringVar(&o.BearerToken, "bearer-token", o.BearerToken, "X API bearer token")
	f.StringVar(&o.TokenFile, "token-file", "", "OAuth2 token JSON file")
	f.StringVar(&o.ClientID, "client-id", o.ClientID, "OAuth2 client ID for refresh")
	f.DurationVar(&o.Timeout, "timeout", o.Timeout, "command timeout")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printSearchUsage(stdout)
			return errHelpRequested
		}
		printSearchUsage(stderr)
		return commandLineError(err.Error())
	}
	if f.NArg() > 0 || o.Query == "" {
		return commandLineError("--query is required and positional arguments are not accepted")
	}
	if o.Limit < 1 || o.Limit > 1000 {
		return commandLineError("--limit must be between 1 and 1000")
	}
	if o.Timeout <= 0 {
		return commandLineError("--timeout must be greater than 0")
	}
	times := map[string]string{"--start-time": o.StartTime, "--end-time": o.EndTime}
	parsedTimes := map[string]time.Time{}
	for name, value := range times {
		if value != "" {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return commandLineError(name + " must be RFC3339")
			}
			parsedTimes[name] = parsed
		}
	}
	if start, ok := parsedTimes["--start-time"]; ok {
		if end, ok := parsedTimes["--end-time"]; ok && !start.Before(end) {
			return commandLineError("--start-time must be before --end-time")
		}
	}
	query := o.Query
	if o.Language != "" {
		query = "(" + query + ") lang:" + o.Language
	}
	commandCtx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	client, err := newAuthenticatedClient(commandCtx, o.TokenFile, o.ClientID, o.BearerToken)
	if err != nil {
		return err
	}
	out := output.SearchOutput{
		Status: "complete",
		Search: output.SearchConditions{
			Query: o.Query, Language: o.Language, StartTime: o.StartTime,
			EndTime: o.EndTime, Limit: o.Limit,
		},
		RetrievedAt: timeNow().UTC().Format(time.RFC3339),
		Posts:       []output.Post{},
	}
	seen := map[string]struct{}{}
	next := ""
	for len(out.Posts) < o.Limit {
		pageSize := o.Limit - len(out.Posts)
		pageSize = max(pageSize, 10)
		pageSize = min(pageSize, 100)
		var page xapi.SearchResponse
		err := client.run(func(api *xapi.Client) error {
			var err error
			page, err = api.SearchRecent(commandCtx, xapi.SearchOptions{
				Query: query, StartTime: o.StartTime, EndTime: o.EndTime,
				NextToken: next, MaxResults: pageSize,
			})
			return err
		})
		if err != nil {
			return finishSearchFailure(o.Output, stdout, out, err)
		}
		authors := make(map[string]xapi.User, len(page.Includes.Users))
		for _, author := range page.Includes.Users {
			authors[author.ID] = author
		}
		pageHadMore := false
		for _, tweet := range page.Data {
			if _, exists := seen[tweet.ID]; exists {
				continue
			}
			if len(out.Posts) >= o.Limit {
				pageHadMore = true
				break
			}
			seen[tweet.ID] = struct{}{}
			out.PostCount++
			postText := tweet.Text
			urls := append([]xapi.TweetURL(nil), tweet.Entities.URLs...)
			if tweet.NotePost != nil {
				if tweet.NotePost.Text != "" {
					postText = tweet.NotePost.Text
				}
				urls = append(urls, tweet.NotePost.Entities.URLs...)
			}
			externalLinks := make([]output.ExternalURL, 0, len(urls))
			for _, link := range urls {
				target := link.UnwoundURL
				if target == "" {
					target = link.ExpandedURL
				}
				if isExternalURL(target) {
					externalLinks = append(externalLinks, output.ExternalURL{
						ShortURL: link.URL, ExpandedURL: target,
					})
				}
			}
			author := authors[tweet.AuthorID]
			username := author.Username
			postURL := "https://x.com/i/status/" + tweet.ID
			if username != "" {
				postURL = "https://x.com/" + username + "/status/" + tweet.ID
			}
			out.Posts = append(out.Posts, output.Post{
				ID: tweet.ID, URL: postURL, Text: postText,
				CreatedAt: tweet.CreatedAt,
				Author: output.PostAuthor{
					ID: tweet.AuthorID, Name: author.Name, Username: username,
				},
				ExternalURLs: externalLinks,
			})
		}
		if len(page.Errors) > 0 {
			return finishSearchAPIError(o.Output, stdout, out, len(page.Errors))
		}
		next = page.Meta.NextToken
		if len(out.Posts) == o.Limit && (next != "" || pageHadMore) {
			out.Status = "limit_reached"
			break
		}
		if next == "" {
			break
		}
	}
	return writeSearchOutput(o.Output, stdout, out)
}

func isExternalURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	return host != "x.com" && !strings.HasSuffix(host, ".x.com") &&
		host != "twitter.com" && !strings.HasSuffix(host, ".twitter.com")
}

func finishSearchFailure(path string, w io.Writer, out output.SearchOutput, err error) error {
	out.Status, out.IncompleteReason = "incomplete", classifySearchError(err)
	if writeErr := writeSearchOutput(path, w, out); writeErr != nil {
		return writeErr
	}
	return fmt.Errorf("search incomplete: %s: %w", out.IncompleteReason, err)
}

func finishSearchAPIError(path string, w io.Writer, out output.SearchOutput, count int) error {
	out.Status, out.IncompleteReason = "incomplete", "api_error"
	if err := writeSearchOutput(path, w, out); err != nil {
		return err
	}
	return fmt.Errorf("search incomplete: api_error: response contained %d API errors", count)
}

func classifySearchError(err error) string {
	var h xapi.HTTPError
	if errors.As(err, &h) {
		if h.StatusCode == http.StatusUnauthorized || h.StatusCode == http.StatusForbidden {
			return "authentication_or_access_denied"
		}
		if h.StatusCode == http.StatusTooManyRequests {
			return "rate_limited"
		}
		return "api_error"
	}
	return "request_failed"
}

func writeSearchOutput(path string, w io.Writer, out output.SearchOutput) error {
	out.PostCount = len(out.Posts)
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path != "" {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		if err := file.Chmod(0o600); err != nil {
			return err
		}
		if err := file.Truncate(0); err != nil {
			return err
		}
		if _, err := file.Write(data); err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		_, err = fmt.Fprintf(
			w, "Saved %d posts to %s (status: %s)\n",
			out.PostCount, path, out.Status,
		)
		return err
	}
	_, err = w.Write(data)
	return err
}

func printSearchUsage(w io.Writer) {
	lines := []string{
		"Usage:", "  xapi-usecase search --query QUERY [options]", "", "Options:",
		"  --query QUERY          X search expression (required)",
		"  --lang CODE            language filter (for example ja or en)",
		"  --start-time RFC3339    oldest post time",
		"  --end-time RFC3339      newest post time",
		"  --limit N               maximum posts (1-1000; default 100)",
		"  --output PATH           save JSON to file",
		"  --bearer-token TOKEN    X API bearer token",
		"  --token-file PATH       OAuth2 token JSON file (defaults to the saved login)",
		"  --client-id CLIENT_ID   OAuth2 client ID for refresh",
		"  --timeout DURATION      command timeout",
		"", "Authentication:",
		"  Uses the token saved by auth login unless a bearer token is provided.",
		"  --bearer-token overrides XAPI_USECASE_BEARER_TOKEN and the saved token.",
		"  --client-id overrides XAPI_USECASE_CLIENT_ID; required only for refresh.",
		"  Required scopes: tweet.read, users.read; offline.access for refresh.",
	}
	_, _ = fmt.Fprintln(w, strings.Join(lines, "\n"))
}
