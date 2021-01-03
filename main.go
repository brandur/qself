package main

import (
	"encoding/xml"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// SyncAllOptions are options that get passed into the `sync-all` command.
type SyncAllOptions struct {
	GoodreadsPath string
	TwitterPath   string
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "qself",
		Short: "Qself syncs personal data from APIs",
		Long: strings.TrimSpace(`
Qself is a small tool to sync personal data from APIs down to
local TOML files for easier portability and storage.`),
	}

	var syncAllOptions SyncAllOptions
	syncAllCommand := &cobra.Command{
		Use:   "sync-all",
		Short: "Sync all qself data",
		Long: strings.TrimSpace(`
Sync all qself data. Individual target files must be set as options..`),
		Run: func(cmd *cobra.Command, args []string) {
			if err := syncAll(&syncAllOptions); err != nil {
				die(fmt.Sprintf("error syncing all: %v", err))
			}
		},
	}
	syncAllCommand.Flags().StringVar(&syncAllOptions.GoodreadsPath,
		"goodreads-path", "PATH", "Goodreads target path")
	syncAllCommand.MarkFlagRequired("goodreads-path")
	syncAllCommand.Flags().StringVar(&syncAllOptions.TwitterPath,
		"twitter-path", "PATH", "Twitter target path")
	syncAllCommand.MarkFlagRequired("twitter-path")
	rootCmd.AddCommand(syncAllCommand)

	syncGoodreadsCommand := &cobra.Command{
		Use:   "sync-goodreads [target TOML file]",
		Short: "Sync Goodreads data",
		Long: strings.TrimSpace(`
Sync personal tweets down from the Goodreads API.`),
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := syncGoodreads(args[0]); err != nil {
				die(fmt.Sprintf("(goodreads) error syncing: %v", err))
			}
		},
	}
	rootCmd.AddCommand(syncGoodreadsCommand)

	syncTwitterCommand := &cobra.Command{
		Use:   "sync-twitter [target TOML file]",
		Short: "Sync Twitter data",
		Long: strings.TrimSpace(`
Sync personal tweets down from the Twitter API.`),
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := syncTwitter(args[0]); err != nil {
				die(fmt.Sprintf("(twitter) error syncing: %v", err))
			}
		},
	}
	rootCmd.AddCommand(syncTwitterCommand)

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

//
// Confs
//

// GoodreadsConf contains configuration information for syncing Goodreads. It's
// extracted from environment variables.
type GoodreadsConf struct {
	GoodreadsID  string `env:"GOODREADS_ID,required"`
	GoodreadsKey string `env:"GOODREADS_KEY,required"`
}

// TwitterConf contains configuration information for syncing Twitter. It's
// extracted from environment variables.
type TwitterConf struct {
	TwitterConsumerKey    string `env:"TWITTER_CONSUMER_KEY,required"`
	TwitterConsumerSecret string `env:"TWITTER_CONSUMER_SECRET,required"`

	TwitterAccessToken  string `env:"TWITTER_ACCESS_TOKEN,required"`
	TwitterAccessSecret string `env:"TWITTER_ACCESS_SECRET,required"`

	TwitterUser string `env:"TWITTER_USER,required"`
}

//
// Goodreads
//

// APIBook is the book nested within a Goodreads review from the API.
type APIBook struct {
	XMLName struct{} `xml:"book"`

	Authors       []*APIBookAuthor `xml:"authors>author"`
	ID            int              `xml:"id"`
	ISBN          string           `xml:"isbn"`
	ISBN13        string           `xml:"isbn13"`
	NumPages      int              `xml:"num_pages"`
	PublishedYear int              `xml:"published"`
	Title         string           `xml:"title"`
}

// APIBookAuthor is an author nested within a Goodreads book from the API.
type APIBookAuthor struct {
	XMLName struct{} `xml:"author"`

	ID   int    `xml:"id"`
	Name string `xml:"name"`
}

// APIReview is a single review within a Goodreads reviews API request.
type APIReview struct {
	XMLName struct{} `xml:"review"`

	Body   string   `xml:"body"`
	Book   *APIBook `xml:"book"`
	ID     int      `xml:"id"`
	Rating int      `xml:"rating"`
	ReadAt string   `xml:"read_at"`
}

// APIReviewsRoot is the root document for a Goodreads reviews API request.
type APIReviewsRoot struct {
	XMLName struct{} `xml:"GoodreadsResponse"`

	Reviews []*APIReview `xml:"reviews>review"`
}

// Reading is a single Goodreads book stored to a TOML file.
type Reading struct {
	Authors       []*ReadingAuthor `toml:"authors"`
	ID            int              `toml:"id"`
	ISBN          string           `toml:"isbn"`
	ISBN13        string           `toml:"isbn13"`
	NumPages      int              `toml:"num_pages"`
	PublishedYear int              `toml:"published_year"`
	ReadAt        time.Time        `toml:"read_at"`
	Rating        int              `toml:"rating"`
	Review        string           `toml:"review"`
	ReviewID      int              `toml:"review_id"`
	Title         string           `toml:"title"`
}

// ReadingAuthor is a single Goodreads author stored to a TOML file.
type ReadingAuthor struct {
	ID   int    `toml:"id"`
	Name string `toml:"name"`
}

// ReadingDB is a database of Goodreads readings stored to a TOML file.
type ReadingDB struct {
	Readings []*Reading `toml:"readings"`
}

//
// Twitter
//

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
	ID   int64  `toml:"id"`
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

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

var htmlLineBreakRE = regexp.MustCompile(`<br ?/?>`)

var htmlLinkRE = regexp.MustCompile(`<a .*?href="(.*?)".*?>.*?</a>`)

func die(message string) {
	fmt.Fprintf(os.Stderr, message)
	os.Exit(1)
}

// Fetches a single Goodreads page and returns all the reviews on it.
func fetchGoodreadsPage(conf *GoodreadsConf, client *http.Client, page int) ([]*APIReview, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://www.goodreads.com/review/list/%s.xml", conf.GoodreadsID), nil)
	if err != nil {
		return nil, err
	}

	v := url.Values{}
	v.Set("key", conf.GoodreadsKey)
	v.Set("page", strconv.Itoa(page))
	v.Set("per_page", "20")
	v.Set("shelf", "read")
	v.Set("sort", "date_read")
	v.Set("v", "2")
	req.URL.RawQuery = v.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error listing reviews: %w", err)
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading body from reviews list: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from Goodreads: %v (%s)", resp.StatusCode, data)
	}

	var root APIReviewsRoot
	err = xml.Unmarshal(data, &root)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling reviews from XML: %w", err)
	}

	return root.Reviews, nil
}

// Because we track a tweet's number of favorites and retweets, a problem with
// the current system is that we update the data file constantly as these
// numbers change trivially. Even if you're not a super popular persona on
// Twitter who's getting new likes and retweets all the time, there's still an
// ambient level of background changes as numbers on old tweets increment or
// decrement by a few at a time. My guess is that it's from people deleting
// their accounts or setting them private, but I'm not sure.
//
// Try to keep the system churning less by preferring the data that we already
// have if the change detected is "trivial", meaning the likes and retweets
// only changed by a small amount.
func flipDuplicateTweetsOnTrivialChanges(tweets []*Tweet) {
	for i, j := 0, 1; j < len(tweets); i, j = i+1, j+1 {
		if tweets[i].ID != tweets[j].ID {
			continue
		}

		if tweets[i].Text != tweets[j].Text {
			continue
		}

		if !reflect.DeepEqual(tweets[i].Entities, tweets[j].Entities) {
			continue
		}

		favoriteDiff := absInt(tweets[i].FavoriteCount - tweets[j].FavoriteCount)
		retweetDiff := absInt(tweets[i].RetweetCount - tweets[j].RetweetCount)

		if favoriteDiff < 3 && retweetDiff < 3 {
			tweets[i], tweets[j] = tweets[j], tweets[i]
		}
	}
}

func syncAll(opts *SyncAllOptions) error {
	var wg sync.WaitGroup

	var goodreadsErr error
	wg.Add(1)
	go func() {
		goodreadsErr = syncGoodreads(opts.GoodreadsPath)
		wg.Done()
	}()

	var twitterErr error
	wg.Add(1)
	go func() {
		twitterErr = syncTwitter(opts.TwitterPath)
		wg.Done()
	}()

	wg.Wait()

	if goodreadsErr != nil {
		return goodreadsErr
	}
	if twitterErr != nil {
		return twitterErr
	}

	return nil
}

func syncGoodreads(targetPath string) error {
	var conf GoodreadsConf
	if err := envdecode.Decode(&conf); err != nil {
		return fmt.Errorf("error decoding conf from env: %v", err)
	}

	var readings []*Reading
	client := &http.Client{}

	// Unluckily, the Goodreads API is _extremely_ slow. Luckily for us, it
	// supports offset based pagination, making it quite easy for us to
	// parallelize.
	const numSegments = 6
	var anyErr error
	var knownEndPage int
	var mutex sync.RWMutex
	var wg sync.WaitGroup
	wg.Add(numSegments)

	for i := 1; i <= numSegments; i++ {
		segmentNum := i

		go func() {
			page := segmentNum

			for {
				logger.Infof("(goodreads) (segment %v) Paging; num readings accumulated: %v, page: %v",
					segmentNum, len(readings), page)

				if knownEndPage != 0 && page >= knownEndPage {
					logger.Infof("(goodreads) (segment %v) Page %v beyond known end of %v; stopping",
						segmentNum, page, knownEndPage)
					break
				}

				apiReviews, err := fetchGoodreadsPage(&conf, client, page)
				if err != nil {
					logger.Errorf("(goodreads) (segment %v) %v", segmentNum, err)
					anyErr = err
					break
				}

				if len(apiReviews) < 1 {
					// If we know this page is beyond bounds, mark it as such
					// to maybe save some API requests.
					mutex.Lock()
					if knownEndPage == 0 || page < knownEndPage {
						logger.Infof("(goodreads) (segment %v) Setting known end page: %v (previously %v)",
							segmentNum, page, knownEndPage)
						knownEndPage = page
					}
					mutex.Unlock()

					break
				}

				var pageReadings []*Reading
				for _, apiReview := range apiReviews {
					pageReadings = append(pageReadings, readingFromAPIReview(apiReview))
				}

				mutex.Lock()
				readings = append(readings, pageReadings...)
				mutex.Unlock()

				page += numSegments
			}

			wg.Done()
		}()
	}

	wg.Wait()

	if anyErr != nil {
		return anyErr
	}

	if _, err := os.Stat(targetPath); err == nil {
		existingData, err := ioutil.ReadFile(targetPath)
		if err != nil {
			return fmt.Errorf("error reading data file: %w", err)
		}

		var existingReadingDB ReadingDB
		err = toml.Unmarshal(existingData, &existingReadingDB)
		if err != nil {
			return fmt.Errorf("error unmarshaling toml: %w", err)
		}

		logger.Infof("(goodreads) Found existing '%v'; attempting merge of %v existing readings(s) with %v current readings(s)",
			targetPath, len(existingReadingDB.Readings), len(readings))

		readings = mergeReadings(readings, existingReadingDB.Readings)
	} else if os.IsNotExist(err) {
		logger.Infof("(goodreads) Existing DB at '%v' not found; starting fresh", targetPath)
	} else {
		return err
	}

	logger.Infof("(goodreads) Writing %v readings(s) to '%s'", len(readings), targetPath)

	readingDB := &ReadingDB{Readings: readings}
	data, err := toml.Marshal(readingDB)
	if err != nil {
		return fmt.Errorf("error marshaling toml: %w", err)
	}

	err = ioutil.WriteFile(targetPath, data, 0644)
	if err != nil {
		return fmt.Errorf("error writing data file: %w", err)
	}

	return nil
}

func syncTwitter(targetPath string) error {
	var conf TwitterConf
	if err := envdecode.Decode(&conf); err != nil {
		return fmt.Errorf("error decoding conf from env: %v", err)
	}

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
	logger.Infof("(twitter) User ID: %v", user.ID)

	var tweets []*Tweet

	var maxTweetID int64 = 0
	for {
		logger.Infof("(twitter) Paging; num tweets accumulated: %v, max tweet ID: %v", len(tweets), maxTweetID)

		apiTweets, _, err := client.Timelines.UserTimeline(&twitter.UserTimelineParams{
			Count:     200, // maximum 200
			MaxID:     maxTweetID,
			TweetMode: "extended", // non-truncated tweet content
			UserID:    user.ID,
		})
		if err != nil {
			return fmt.Errorf("error listing user timeline: %w", err)
		}

		processedAnyTweets := false

		for _, apiTweet := range apiTweets {
			// Each page contains the last item from the previous page, so skip
			// that
			if maxTweetID != 0 && apiTweet.ID >= maxTweetID {
				continue
			}

			processedAnyTweets = true
			tweets = append(tweets, tweetFromAPITweet(&apiTweet))
		}

		// No suitable tweets on the page to process which means that we're
		// done pagination. Break out of the loop and finish.
		if !processedAnyTweets {
			break
		}

		maxTweetID = apiTweets[len(apiTweets)-1].ID
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

		logger.Infof("(twitter) Found existing '%v'; attempting merge of %v existing tweet(s) with %v current tweet(s)",
			targetPath, len(existingTweetDB.Tweets), len(tweets))

		tweets = mergeTweets(tweets, existingTweetDB.Tweets)
	} else if os.IsNotExist(err) {
		logger.Infof("(twitter) Existing DB at '%v' not found; starting fresh", targetPath)
	} else {
		return err
	}

	logger.Infof("(twitter) Writing %v tweet(s) to '%s'", len(tweets), targetPath)

	tweetDB := &TweetDB{Tweets: tweets}
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

func tweetFromAPITweet(tweet *twitter.Tweet) *Tweet {
	createdAt, err := tweet.CreatedAtTime()
	if err != nil {
		panic(err)
	}

	var entities *TweetEntities

	// The Twitter API is weird. "Extended" entities and entities are almost
	// the same, except that the extended version will contain more than one
	// photo where multiple were included, and it will assign a type that isn't
	// "photo" for videos.
	//
	// Twitter doesn't go into specifics on why extended is even a thing, but
	// it's probably related to the fact that features like videos and multiple
	// photo uploads were added later on.
	if tweet.ExtendedEntities != nil && len(tweet.ExtendedEntities.Media) > 0 {
		if entities == nil {
			entities = &TweetEntities{}
		}

		for _, media := range tweet.ExtendedEntities.Media {
			entities.Medias = append(entities.Medias, &TweetEntitiesMedia{
				ID:   media.ID,
				Type: media.Type,
				URL:  media.MediaURLHttps,
			})
		}
	} else if len(tweet.Entities.Media) > 0 {
		if entities == nil {
			entities = &TweetEntities{}
		}

		for _, media := range tweet.Entities.Media {
			entities.Medias = append(entities.Medias, &TweetEntitiesMedia{
				ID:   media.ID,
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
		Text:          sanitizeTweetText(tweet.FullText),
	}
}

// Merge two sets of readings together.
//
// The first slice should be new readings from the Goodreads API, the second
// should be any existing readings. This matters because we remove any readings
// in the existing set which are no longer in the API (because that means they
// were deleted).
//
// This function is currently extreme overkill at the moment because, unlike
// with Twitter, we never really keep anything from the existing set,
// preferring what's in the API in all cases. I'm leaving it in for now because
// it doesn't matter, and also I may want to alter this behavior at some point.
func mergeReadings(apiReadings, existingReadings []*Reading) []*Reading {
	existingReadings = sliceKeepOnly(existingReadings, apiReadings,
		func(i int) interface{} { return existingReadings[i].ReviewID },
		func(i int) interface{} { return apiReadings[i].ReviewID },
	).([]*Reading)
	s := append(apiReadings, existingReadings...)
	sort.SliceStable(s, func(i, j int) bool { return s[i].ReviewID < s[j].ReviewID })
	sMerged := sliceUniq(s, func(i int) interface{} { return s[i].ReviewID }).([]*Reading)
	sliceReverse(sMerged)
	return sMerged
}

func mergeTweets(apiTweets, existingTweets []*Tweet) []*Tweet {
	s := append(apiTweets, existingTweets...)
	sort.SliceStable(s, func(i, j int) bool { return s[i].ID < s[j].ID })
	flipDuplicateTweetsOnTrivialChanges(s)
	sMerged := sliceUniq(s, func(i int) interface{} { return s[i].ID }).([]*Tweet)
	sliceReverse(sMerged)
	return sMerged
}

// Format which Goodreads returns time in implemented as a Go magic time
// parsing string.
const goodreadsTimeFormat = "Mon Jan 2 15:04:05 -0700 2006"

func readingFromAPIReview(review *APIReview) *Reading {
	var authors []*ReadingAuthor
	for _, author := range review.Book.Authors {
		authors = append(authors, &ReadingAuthor{
			ID:   author.ID,
			Name: author.Name,
		})
	}

	var readAt time.Time
	if review.ReadAt != "" {
		t, err := time.Parse(goodreadsTimeFormat, review.ReadAt)
		if err != nil {
			panic(err)
		}
		readAt = t
	} else {
		logger.Errorf("No read at time for book: %v", review.Book.Title)
	}

	return &Reading{
		Authors:       authors,
		ID:            review.Book.ID,
		ISBN:          review.Book.ISBN,
		ISBN13:        review.Book.ISBN13,
		NumPages:      review.Book.NumPages,
		PublishedYear: review.Book.PublishedYear,
		ReadAt:        readAt,
		Rating:        review.Rating,
		Review:        sanitizeGoodreadsReview(review.Body),
		ReviewID:      review.ID,
		Title:         review.Book.Title,
	}
}

// Goodreads doesn't do a great job of keeping review bodies clean, and does
// things like add HTML line breaks where the user has inserted newlines. Take
// these out and leave the review looking roughly Markdown-esque.
func sanitizeGoodreadsReview(review string) string {
	review = htmlLineBreakRE.ReplaceAllString(review, "\n")
	review = htmlLinkRE.ReplaceAllString(review, "$1")

	review = html.UnescapeString(review)

	return strings.TrimSpace(review)
}

// Clean up anything from Twitter for tweet bodies.
func sanitizeTweetText(text string) string {
	return html.UnescapeString(text)
}

func sliceReverse(s interface{}) {
	n := reflect.ValueOf(s).Len()
	swap := reflect.Swapper(s)
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		swap(i, j)
	}
}

// Keeps only items in s1 which are also available in s2.
func sliceKeepOnly(s1, s2 interface{}, selector1 func(int) interface{}, selector2 func(int) interface{}) interface{} {
	s1Slice := reflect.ValueOf(s1)
	s2Slice := reflect.ValueOf(s2)

	s2Values := make(map[interface{}]struct{})
	for i := 0; i < s2Slice.Len(); i++ {
		value := selector2(i)
		s2Values[value] = struct{}{}
	}

	j := 0

	for i := 0; i < s1Slice.Len(); i++ {
		item := s1Slice.Index(i)
		value := selector1(i)

		if _, ok := s2Values[value]; !ok {
			continue
		}

		s1Slice.Index(j).Set(item)
		j++
	}

	return s1Slice.Slice(0, j).Interface()
}

func sliceUniq(s interface{}, selector func(int) interface{}) interface{} {
	sSlice := reflect.ValueOf(s)

	seen := make(map[interface{}]struct{})
	j := 0

	for i := 0; i < sSlice.Len(); i++ {
		item := sSlice.Index(i)
		uniqValue := selector(i)

		if _, ok := seen[uniqValue]; ok {
			continue
		}

		seen[uniqValue] = struct{}{}
		sSlice.Index(j).Set(item)
		j++
	}

	return sSlice.Slice(0, j).Interface()
}
