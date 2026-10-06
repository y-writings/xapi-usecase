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

	"github.com/y-writings/xapi-usecase/internal/xapi"
)

const bearerTokenEnv = "XAPI_USECASE_BEARER_TOKEN"

type searchOptions struct {
	Query, Language, StartTime, EndTime, Output, BearerToken string
	Limit                                                    int
	Timeout                                                  time.Duration
}
type searchOutput struct {
	Status           string           `json:"status"`
	IncompleteReason string           `json:"incomplete_reason,omitempty"`
	Search           searchConditions `json:"search"`
	RetrievedAt      string           `json:"retrieved_at"`
	PostCount        int              `json:"post_count"`
	ResourceCount    int              `json:"resource_count"`
	Resources        []resource       `json:"resources"`
}
type searchConditions struct {
	Query     string `json:"query"`
	Language  string `json:"language,omitempty"`
	StartTime string `json:"start_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`
	Limit     int    `json:"limit"`
}
type resource struct {
	URL     string   `json:"url"`
	Sources []source `json:"sources"`
}
type source struct {
	PostID    string `json:"post_id"`
	PostURL   string `json:"post_url"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
	ShortURL  string `json:"short_url"`
}

func search(ctx context.Context, args []string, stdout, stderr io.Writer, getenv getenvFunc) error {
	o := searchOptions{Limit: 100, Timeout: 30 * time.Second, BearerToken: getenv(bearerTokenEnv)}
	f := flag.NewFlagSet("xapi-usecase search", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.Query, "query", "", "X search query")
	f.StringVar(&o.Language, "lang", "", "language code")
	f.StringVar(&o.StartTime, "start-time", "", "RFC3339 start time")
	f.StringVar(&o.EndTime, "end-time", "", "RFC3339 end time")
	f.IntVar(&o.Limit, "limit", o.Limit, "maximum posts to retrieve")
	f.StringVar(&o.Output, "output", "", "JSON output file")
	f.StringVar(&o.BearerToken, "bearer-token", o.BearerToken, "X API bearer token")
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
	if o.BearerToken == "" {
		return commandLineError(
			"bearer token is required; set " + bearerTokenEnv + " or pass --bearer-token",
		)
	}
	if o.Limit < 1 || o.Limit > 1000 {
		return commandLineError("--limit must be between 1 and 1000")
	}
	if o.Timeout <= 0 {
		return commandLineError("--timeout must be greater than 0")
	}
	times := map[string]string{"--start-time": o.StartTime, "--end-time": o.EndTime}
	for name, value := range times {
		if value != "" {
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				return commandLineError(name + " must be RFC3339")
			}
		}
	}
	query := o.Query
	if o.Language != "" {
		query = "(" + query + ") lang:" + o.Language
	}
	commandCtx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	client := newXAPIClient(o.BearerToken)
	out := searchOutput{
		Status: "complete",
		Search: searchConditions{
			Query: o.Query, Language: o.Language, StartTime: o.StartTime,
			EndTime: o.EndTime, Limit: o.Limit,
		},
		RetrievedAt: timeNow().UTC().Format(time.RFC3339),
		Resources:   []resource{},
	}
	byURL := map[string]int{}
	next := ""
	for out.PostCount < o.Limit {
		pageSize := o.Limit - out.PostCount
		pageSize = max(pageSize, 10)
		pageSize = min(pageSize, 100)
		page, err := client.SearchRecent(commandCtx, xapi.SearchOptions{
			Query: query, StartTime: o.StartTime, EndTime: o.EndTime,
			NextToken: next, MaxResults: pageSize,
		})
		if err != nil {
			out.Status = "incomplete"
			out.IncompleteReason = classifySearchError(err)
			if writeErr := writeSearchOutput(o.Output, stdout, out); writeErr != nil {
				return writeErr
			}
			return fmt.Errorf("search incomplete: %s: %w", out.IncompleteReason, err)
		}
		for _, tweet := range page.Data {
			if out.PostCount >= o.Limit {
				break
			}
			out.PostCount++
			for _, link := range tweet.Entities.URLs {
				target := link.UnwoundURL
				if target == "" {
					target = link.ExpandedURL
				}
				canonical, ok := externalURL(target)
				if !ok {
					continue
				}
				s := source{
					PostID: tweet.ID, PostURL: "https://x.com/i/status/" + tweet.ID,
					Text: tweet.Text, CreatedAt: tweet.CreatedAt, ShortURL: link.URL,
				}
				idx, exists := byURL[canonical]
				if !exists {
					idx = len(out.Resources)
					byURL[canonical] = idx
					out.Resources = append(out.Resources, resource{
						URL: canonical, Sources: []source{},
					})
				}
				out.Resources[idx].Sources = append(out.Resources[idx].Sources, s)
			}
		}
		next = page.Meta.NextToken
		if next == "" {
			break
		}
	}
	out.ResourceCount = len(out.Resources)
	return writeSearchOutput(o.Output, stdout, out)
}

func externalURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", false
	}
	h := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	isX := h == "x.com" || strings.HasSuffix(h, ".x.com")
	isTwitter := h == "twitter.com" || strings.HasSuffix(h, ".twitter.com")
	if isX || isTwitter {
		return "", false
	}
	u.Fragment = ""
	return u.String(), true
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

func writeSearchOutput(path string, w io.Writer, out searchOutput) error {
	out.ResourceCount = len(out.Resources)
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
			w, "Saved %d resources to %s (status: %s)\n",
			out.ResourceCount, path, out.Status,
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
		"  --timeout DURATION      command timeout",
	}
	_, _ = fmt.Fprintln(w, strings.Join(lines, "\n"))
}
