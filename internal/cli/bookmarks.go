package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/y-writings/xapi-usecase/internal/xapi"
)

const (
	defaultBookmarksTimeout     = 30 * time.Second
	defaultBookmarkTweetFields  = "created_at,author_id"
	maxBookmarkResultsInclusive = 100
)

type bookmarksListOptions struct {
	ClientID        string
	TokenFile       string
	MaxResults      int
	PaginationToken string
	TweetFields     string
	Expansions      string
	UserFields      string
	MediaFields     string
	PollFields      string
	PlaceFields     string
	Timeout         time.Duration
}

func bookmarksList(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	getenv getenvFunc,
) error {
	options, err := parseBookmarksListOptions(args, stdout, stderr, getenv)
	if err != nil {
		return err
	}

	commandCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	client, err := newAuthenticatedClient(commandCtx, options.TokenFile, options.ClientID, "")
	if err != nil {
		return err
	}

	var me xapi.MeResponse
	if err := client.run(func(api *xapi.Client) error {
		var err error
		me, err = api.Me(commandCtx)
		return err
	}); err != nil {
		return err
	}

	var raw []byte
	if err := client.run(func(api *xapi.Client) error {
		var err error
		raw, err = api.BookmarksRaw(commandCtx, me.Data.ID, xapi.BookmarkOptions{
			MaxResults:      options.MaxResults,
			PaginationToken: options.PaginationToken,
			TweetFields:     options.TweetFields,
			Expansions:      options.Expansions,
			UserFields:      options.UserFields,
			MediaFields:     options.MediaFields,
			PollFields:      options.PollFields,
			PlaceFields:     options.PlaceFields,
		})
		return err
	}); err != nil {
		return err
	}

	return writePrettyJSON(stdout, raw)
}

func parseBookmarksListOptions(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	getenv getenvFunc,
) (bookmarksListOptions, error) {
	options := bookmarksListOptions{
		ClientID:    getenv(clientIDEnv),
		TweetFields: defaultBookmarkTweetFields,
		Timeout:     defaultBookmarksTimeout,
	}

	flags := flag.NewFlagSet("xapi-usecase bookmarks list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.ClientID, "client-id", options.ClientID, "OAuth2 client ID")
	flags.StringVar(&options.TokenFile, "token-file", options.TokenFile, "OAuth2 token path")
	flags.IntVar(&options.MaxResults, "max-results", options.MaxResults, "results per page")
	flags.StringVar(
		&options.PaginationToken,
		"pagination-token",
		options.PaginationToken,
		"page token",
	)
	flags.StringVar(&options.TweetFields, "tweet-fields", options.TweetFields, "tweet.fields")
	flags.StringVar(&options.Expansions, "expansions", options.Expansions, "expansions")
	flags.StringVar(&options.UserFields, "user-fields", options.UserFields, "user.fields")
	flags.StringVar(&options.MediaFields, "media-fields", options.MediaFields, "media.fields")
	flags.StringVar(&options.PollFields, "poll-fields", options.PollFields, "poll.fields")
	flags.StringVar(&options.PlaceFields, "place-fields", options.PlaceFields, "place.fields")
	flags.DurationVar(&options.Timeout, "timeout", options.Timeout, "command timeout")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printBookmarksListUsage(stdout)
			return bookmarksListOptions{}, errHelpRequested
		}
		printBookmarksListUsage(stderr)
		return bookmarksListOptions{}, commandLineError(err.Error())
	}
	if flags.NArg() > 0 {
		printBookmarksListUsage(stderr)
		return bookmarksListOptions{}, commandLineError(
			fmt.Sprintf("unexpected argument: %s", flags.Arg(0)),
		)
	}
	if flagIsSet(flags, "max-results") &&
		(options.MaxResults < 1 || options.MaxResults > maxBookmarkResultsInclusive) {
		return bookmarksListOptions{}, commandLineError("--max-results must be between 1 and 100")
	}
	if options.Timeout <= 0 {
		return bookmarksListOptions{}, commandLineError("--timeout must be greater than 0")
	}
	return options, nil
}

func flagIsSet(flags *flag.FlagSet, name string) bool {
	set := false
	flags.Visit(func(flag *flag.Flag) {
		if flag.Name == name {
			set = true
		}
	})
	return set
}

func writePrettyJSON(w io.Writer, raw []byte) error {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return err
	}
	if _, err := pretty.WriteTo(w); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w)
	return err
}
