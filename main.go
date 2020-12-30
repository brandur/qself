package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/joeshaw/envdecode"
	"github.com/pelletier/go-toml"
	"github.com/spf13/cobra"
)

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Main
//
//
//
//////////////////////////////////////////////////////////////////////////////

func main() {
	var rootCmd = &cobra.Command{
		Use:   "qself",
		Short: "Qself syncs personal data from APIs",
		Long: strings.TrimSpace(`
Qself is a small tool to sync personal data from APIs down to
local TOML files for easier portability and storage.`),
	}

	syncTwitterCommand := &cobra.Command{
		Use:   "sync-twitter [target TOML file]",
		Short: "Sync Twitter data",
		Long: strings.TrimSpace(`
Sync personal tweets down from the twitter API.`),
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := syncTwitter(args[0]); err != nil {
				die(fmt.Sprintf("error syncing twitter: %v", err))
			}
		},
	}
	rootCmd.AddCommand(syncTwitterCommand)

	if err := envdecode.Decode(&conf); err != nil {
		die(fmt.Sprintf("Error decoding conf from env: %v", err))
	}

	if err := rootCmd.Execute(); err != nil {
		die(fmt.Sprintf("Error executing command: %v", err))
	}
}

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Types
//
//
//
//////////////////////////////////////////////////////////////////////////////

// Conf contains configuration information for the command. It's extracted from
// environment variables.
type Conf struct {
	TwitterConsumerKey    string `env:"TWITTER_CONSUMER_KEY,required"`
	TwitterConsumerSecret string `env:"TWITTER_CONSUMER_SECRET,required"`

	TwitterAccessToken  string `env:"TWITTER_ACCESS_TOKEN,required"`
	TwitterAccessSecret string `env:"TWITTER_ACCESS_SECRET,required"`

	TwitterUser string `env:"TWITTER_USER,required"`
}

// TweetDB is a database of tweets stored to a TOML file.
type TweetDB struct {
	Tweets []*Tweet `toml:"tweets"`
}

// Tweet is a single tweet stored to a TOML file.
type Tweet struct {
	CreatedAt     time.Time      `toml:"created_at"`
	Entities      *TweetEntities `toml:"entities"`
	FavoriteCount int            `toml:"favorite_count,omitempty"`
	ID            int64          `toml:"id"`
	Reply         *TweetReply    `toml:"reply"`
	Retweet       *TweetRetweet  `toml:"retweet"`
	RetweetCount  int            `toml:"retweet_count,omitempty"`
	Text          string         `toml:"text"`
}

// TweetEntities contains various multimedia entries that may be contained in a
// tweet.
type TweetEntities struct {
	Medias       []*TweetEntitiesMedia       `toml:"medias"`
	URLs         []*TweetEntitiesURL         `toml:"urls"`
	UserMentions []*TweetEntitiesUserMention `toml:"user_mentions"`
}

// TweetEntitiesMedia is an image or video stored in a tweet.
type TweetEntitiesMedia struct {
	Type string `toml:"type"`
	URL  string `toml:"url"`
}

// TweetEntitiesURL is a URL referenced in a tweet.
type TweetEntitiesURL struct {
	DisplayURL  string `toml:"display_url"`
	ExpandedURL string `toml:"expanded_url"`
	URL         string `toml:"url"`
}

// TweetEntitiesUserMention is another user being mentioned in a tweet.
type TweetEntitiesUserMention struct {
	User   string `toml:"user"`
	UserID int64  `toml:"user_id"`
}

// TweetReply is populated with reply information for when a tweet is a
// reply.
type TweetReply struct {
	StatusID int64  `toml:"status_id"`
	User     string `toml:"user"`
	UserID   int64  `toml:"user_id"`
}

// TweetRetweet is populated with retweet information for when a tweet is a
// retweet.
type TweetRetweet struct {
	StatusID int64  `toml:"status_id"`
	User     string `toml:"user"`
	UserID   int64  `toml:"user_id"`
}

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Variables
//
//
//
//////////////////////////////////////////////////////////////////////////////

// Left as a global for now for the sake of convenience, but it's not used in
// very many places and can probably be refactored as a local if desired.
var conf Conf

var logger = &LeveledLogger{Level: LevelInfo}

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Private functions
//
//
//
//////////////////////////////////////////////////////////////////////////////

func die(message string) {
	fmt.Fprintf(os.Stderr, message)
	os.Exit(1)
}

func syncTwitter(targetPath string) error {
	config := oauth1.NewConfig(conf.TwitterConsumerKey, conf.TwitterConsumerSecret)
	token := oauth1.NewToken(conf.TwitterAccessToken, conf.TwitterAccessSecret)
	httpClient := config.Client(oauth1.NoContext, token)

	client := twitter.NewClient(httpClient)

	user, _, err := client.Users.Show(&twitter.UserShowParams{
		ScreenName: conf.TwitterUser,
	})
	if err != nil {
		return fmt.Errorf("error getting user '%v': %w", conf.TwitterUser, err)
	}
	logger.Infof("Twitter user ID: %v", user.ID)

	var tweetDatas []*Tweet

	var maxTweetID int64 = 0
	for {
		logger.Infof("Paging; num tweets accumulated: %v, max tweet ID: %v", len(tweetDatas), maxTweetID)

		tweets, _, err := client.Timelines.UserTimeline(&twitter.UserTimelineParams{
			Count:     200, // maximum 200
			MaxID:     maxTweetID,
			TweetMode: "extended", // non-truncated tweet content
			UserID:    user.ID,
		})
		if err != nil {
			return fmt.Errorf("error listing user timeline: %w", err)
		}

		processedAnyTweets := false

		for _, tweet := range tweets {
			// Each page contains the last item from the previous page, so skip
			// that
			if maxTweetID != 0 && tweet.ID >= maxTweetID {
				continue
			}

			processedAnyTweets = true
			tweetDatas = append(tweetDatas, tweetDataFromAPITweet(&tweet))
		}

		// No suitable tweets on the page to process which means that we're
		// done pagination. Break out of the loop and finish.
		if !processedAnyTweets {
			break
		}

		maxTweetID = tweets[len(tweets)-1].ID
	}

	// Twitter returns a maximum of ~3200 tweets ever, so try to maintain older
	// ones by merging any existing data that we already have.
	if _, err := os.Stat(targetPath); err == nil {
		existingData, err := ioutil.ReadFile(targetPath)
		if err != nil {
			return fmt.Errorf("error reading data file: %w", err)
		}

		var existingTweetDB TweetDB
		err = toml.Unmarshal(existingData, &existingTweetDB)
		if err != nil {
			return fmt.Errorf("error unmarshaling toml: %w", err)
		}

		logger.Infof("Found existing '%v'; attempting merge of %v existing tweet(s) with %v current tweet(s)",
			targetPath, len(existingTweetDB.Tweets), len(tweetDatas))

		tweetDatas = mergeTweets(tweetDatas, existingTweetDB.Tweets)
	} else if os.IsNotExist(err) {
		logger.Infof("Existing DB at '%v' not found; starting fresh", targetPath)
	} else {
		return err
	}

	logger.Infof("Writing %v tweet(s) to '%s'", len(tweetDatas), targetPath)

	tweetDB := &TweetDB{Tweets: tweetDatas}
	data, err := toml.Marshal(tweetDB)
	if err != nil {
		return fmt.Errorf("error marshaling toml: %w", err)
	}

	err = ioutil.WriteFile(targetPath, data, 0644)
	if err != nil {
		return fmt.Errorf("error writing data file: %w", err)
	}

	return nil
}

func tweetDataFromAPITweet(tweet *twitter.Tweet) *Tweet {
	createdAt, err := tweet.CreatedAtTime()
	if err != nil {
		panic(err)
	}

	var entities *TweetEntities

	// For some reason this only includes a single photo given a
	// multi-photo tweet. A project for another day though ...
	if len(tweet.Entities.Media) > 0 {
		if entities == nil {
			entities = &TweetEntities{}
		}

		for _, media := range tweet.Entities.Media {
			entities.Medias = append(entities.Medias, &TweetEntitiesMedia{
				Type: media.Type,
				URL:  media.MediaURLHttps,
			})
		}
	}

	if len(tweet.Entities.Urls) > 0 {
		if entities == nil {
			entities = &TweetEntities{}
		}

		for _, url := range tweet.Entities.Urls {
			entities.URLs = append(entities.URLs, &TweetEntitiesURL{
				DisplayURL:  url.DisplayURL,
				ExpandedURL: url.ExpandedURL,
				URL:         url.URL,
			})
		}
	}

	if len(tweet.Entities.UserMentions) > 0 {
		if entities == nil {
			entities = &TweetEntities{}
		}

		for _, userMention := range tweet.Entities.UserMentions {
			entities.UserMentions = append(entities.UserMentions, &TweetEntitiesUserMention{
				User:   userMention.ScreenName,
				UserID: userMention.ID,
			})
		}
	}

	var reply *TweetReply
	if tweet.InReplyToStatusID != 0 {
		reply = &TweetReply{
			StatusID: tweet.InReplyToStatusID,
			User:     tweet.InReplyToScreenName,
			UserID:   tweet.InReplyToUserID,
		}
	}

	var retweet *TweetRetweet
	if status := tweet.RetweetedStatus; status != nil {
		retweet = &TweetRetweet{
			StatusID: status.ID,
			User:     status.User.ScreenName,
			UserID:   status.User.ID,
		}
	}

	return &Tweet{
		CreatedAt:     createdAt,
		Entities:      entities,
		FavoriteCount: tweet.FavoriteCount,
		ID:            tweet.ID,
		Reply:         reply,
		Retweet:       retweet,
		RetweetCount:  tweet.RetweetCount,
		Text:          tweet.FullText,
	}
}

func mergeTweets(s1, s2 []*Tweet) []*Tweet {
	s := append(s1, s2...)
	sort.SliceStable(s, func(i, j int) bool { return s[i].ID < s[j].ID })
	sMerged := sliceUniq(s, func(i int) interface{} { return s[i].ID }).([]*Tweet)
	sliceReverse(sMerged)
	return sMerged
}

func sliceReverse(s interface{}) {
	n := reflect.ValueOf(s).Len()
	swap := reflect.Swapper(s)
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		swap(i, j)
	}
}

func sliceUniq(s interface{}, selector func(int) interface{}) interface{} {
	sValue := reflect.ValueOf(s)

	seen := make(map[interface{}]struct{})
	j := 0

	for i := 0; i < sValue.Len(); i++ {
		value := sValue.Index(i)
		uniqValue := selector(i)

		if _, ok := seen[uniqValue]; ok {
			continue
		}

		seen[uniqValue] = struct{}{}
		sValue.Index(j).Set(value)
		j++
	}

	return sValue.Slice(0, j).Interface()
}
