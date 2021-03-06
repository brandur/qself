package main

import (
	"testing"

	assert "github.com/stretchr/testify/require"
)

func TestMergeReadings(t *testing.T) {
	t.Run("Standard", func(t *testing.T) {
		s1 := []*Reading{
			{ReviewID: 125, Review: "s1 125"},
			{ReviewID: 124, Review: "s1 124"},
			{ReviewID: 123, Review: "s1 123"},
			{ReviewID: 122, Review: "s1 122"},
		}
		s2 := []*Reading{
			{ReviewID: 124, Review: "s2 124"},
			{ReviewID: 123, Review: "s2 123"},
		}

		s := mergeReadings(s1, s2)

		assert.Equal(
			t,
			[]*Reading{
				{ReviewID: 125, Review: "s1 125"},
				{ReviewID: 124, Review: "s1 124"}, // s1 is preferred
				{ReviewID: 123, Review: "s1 123"}, // s1 is preferred
				{ReviewID: 122, Review: "s1 122"},
			},
			s,
		)
	})

	t.Run("RemoveOld", func(t *testing.T) {
		s1 := []*Reading{
			{ReviewID: 125},
			{ReviewID: 123},
		}
		s2 := []*Reading{
			{ReviewID: 125},
			{ReviewID: 124},
			{ReviewID: 123},
		}

		s := mergeReadings(s1, s2)

		assert.Equal(
			t,
			[]*Reading{
				{ReviewID: 125},
				{ReviewID: 123},
			},
			s,
		)
	})
}

func TestMergeTweets(t *testing.T) {
	t.Run("Standard", func(t *testing.T) {
		s1 := []*Tweet{
			{ID: 125, Text: "s1 125"},
			{ID: 124, Text: "s1 124"},
			{ID: 122, Text: "s1 122"},
		}
		s2 := []*Tweet{
			{ID: 124, Text: "s2 124"},
			{ID: 123, Text: "s2 123"},
			{ID: 121, Text: "s2 121"},
		}

		s := mergeTweets(s1, s2)

		assert.Equal(
			t,
			[]*Tweet{
				{ID: 125, Text: "s1 125"},
				{ID: 124, Text: "s1 124"}, // s1 is preferred
				{ID: 123, Text: "s2 123"},
				{ID: 122, Text: "s1 122"},
				{ID: 121, Text: "s2 121"},
			},
			s,
		)
	})

	t.Run("NewPreferredOnNonTrivialChangesInc", func(t *testing.T) {
		s1 := []*Tweet{
			{ID: 125, Text: "s1 125"},
			// Text should be the same
			{ID: 124, Text: "sX 124", FavoriteCount: 10, RetweetCount: 10},
		}
		s2 := []*Tweet{
			{ID: 124, Text: "sX 124", FavoriteCount: 2, RetweetCount: 2},
			{ID: 123, Text: "s2 123"},
		}

		s := mergeTweets(s1, s2)

		assert.Equal(
			t,
			[]*Tweet{
				{ID: 125, Text: "s1 125"},
				{ID: 124, Text: "sX 124", FavoriteCount: 10, RetweetCount: 10}, // s1 is preferred
				{ID: 123, Text: "s2 123"},
			},
			s,
		)
	})

	t.Run("NewPreferredOnNonTrivialChangesDec", func(t *testing.T) {
		s1 := []*Tweet{
			{ID: 125, Text: "s1 125"},
			// Text should be the same
			{ID: 124, Text: "sX 124", FavoriteCount: 2, RetweetCount: 2},
		}
		s2 := []*Tweet{
			{ID: 124, Text: "sX 124", FavoriteCount: 10, RetweetCount: 10},
			{ID: 123, Text: "s2 123"},
		}

		s := mergeTweets(s1, s2)

		assert.Equal(
			t,
			[]*Tweet{
				{ID: 125, Text: "s1 125"},
				{ID: 124, Text: "sX 124", FavoriteCount: 2, RetweetCount: 2}, // s1 is preferred
				{ID: 123, Text: "s2 123"},
			},
			s,
		)
	})

	t.Run("OldPreferredOnTrivialChanges", func(t *testing.T) {
		s1 := []*Tweet{
			{ID: 125, Text: "s1 125"},
			// Text must be the same for this to work
			{ID: 124, Text: "sX 124", FavoriteCount: 4, RetweetCount: 4},
		}
		s2 := []*Tweet{
			{ID: 124, Text: "sX 124", FavoriteCount: 2, RetweetCount: 2},
			{ID: 123, Text: "s2 123"},
		}

		s := mergeTweets(s1, s2)

		assert.Equal(
			t,
			[]*Tweet{
				{ID: 125, Text: "s1 125"},
				{ID: 124, Text: "sX 124", FavoriteCount: 2, RetweetCount: 2}, // s2 is preferred
				{ID: 123, Text: "s2 123"},
			},
			s,
		)
	})

	t.Run("NewPreferredOnTrivialChangesIfEntitiesDifferent", func(t *testing.T) {
		s1 := []*Tweet{
			{ID: 125, Text: "s1 125"},
			{ID: 124, Text: "sX 124", FavoriteCount: 4, RetweetCount: 4,
				Entities: &TweetEntities{Medias: []*TweetEntitiesMedia{{URL: "https://foo.com"}}}},
		}
		s2 := []*Tweet{
			{ID: 124, Text: "sX 124", FavoriteCount: 2, RetweetCount: 2,
				Entities: &TweetEntities{Medias: []*TweetEntitiesMedia{{URL: "https://bar.com"}}}},
			{ID: 123, Text: "s2 123"},
		}

		s := mergeTweets(s1, s2)

		assert.Equal(
			t,
			[]*Tweet{
				{ID: 125, Text: "s1 125"},
				{ID: 124, Text: "sX 124", FavoriteCount: 4, RetweetCount: 4,
					Entities: &TweetEntities{Medias: []*TweetEntitiesMedia{{URL: "https://foo.com"}}}},
				{ID: 123, Text: "s2 123"},
			},
			s,
		)
	})
}

func TestSanitizeGoodreadsReview(t *testing.T) {
	assert.Equal(t, "hello", sanitizeGoodreadsReview("hello"))
	assert.Equal(t, "hello", sanitizeGoodreadsReview("   hello   "))
	assert.Equal(t, "hel lo", sanitizeGoodreadsReview("   hel lo   "))

	assert.Equal(t, "hello", sanitizeGoodreadsReview("hello<br>"))
	assert.Equal(t, "hello", sanitizeGoodreadsReview("hello<br><br>"))
	assert.Equal(t, "hello", sanitizeGoodreadsReview("hello<br >"))
	assert.Equal(t, "hello", sanitizeGoodreadsReview("hello<br/>"))
	assert.Equal(t, "hello", sanitizeGoodreadsReview("hello<br />"))

	assert.Equal(
		t,
		"http://example.com/hello/there",
		sanitizeGoodreadsReview(`<a href="http://example.com/hello/there">anything</a>`),
	)

	assert.Equal(
		t,
		"http://example.com/hello/there",
		sanitizeGoodreadsReview(`<a target="_blank" href="http://example.com/hello/there">anything</a>`),
	)

	assert.Equal(
		t,
		"http://example.com/hello/there",
		sanitizeGoodreadsReview(`<a href="http://example.com/hello/there" target="_blank">anything</a>`),
	)

	assert.Equal(
		t,
		"link to http://example.com/hello/there here",
		sanitizeGoodreadsReview(`link to <a href="http://example.com/hello/there">anything</a> here`),
	)

	assert.Equal(
		t,
		"http://example.com/hello/there http://example.com/hello/there",
		sanitizeGoodreadsReview(`<a href="http://example.com/hello/there">anything</a> <a href="http://example.com/hello/there">anything</a>`),
	)

	assert.Equal(
		t,
		"http://example.com/hello/there?a=b&c=d",
		sanitizeGoodreadsReview(`<a href="http://example.com/hello/there?a=b&amp;c=d">anything</a>`),
	)
}

func TestSanitizeTweetText(t *testing.T) {
	assert.Equal(t, "hello", sanitizeTweetText("hello"))
	assert.Equal(t, "<tag>", sanitizeTweetText("<tag>"))
	assert.Equal(t, "<tag>", sanitizeTweetText("&lt;tag&gt;"))
}

func TestSliceReverse(t *testing.T) {
	s := []int{1, 2, 3}
	sliceReverse(s)

	assert.Equal(
		t,
		[]int{3, 2, 1},
		s,
	)
}

func TestSliceKeepOnly(t *testing.T) {
	s1 := []int{1, 2, 3, 4}
	s2 := []int{2, 3, 5}
	s1 = sliceKeepOnly(s1, s2,
		func(i int) interface{} { return s1[i] },
		func(i int) interface{} { return s2[i] },
	).([]int)

	assert.Equal(
		t,
		[]int{2, 3},
		s1,
	)
}

func TestSliceUniq(t *testing.T) {
	s := []int{1, 2, 2, 3, 4, 5, 6, 7, 7, 7, 8, 9}
	s = sliceUniq(s, func(i int) interface{} { return s[i] }).([]int)

	assert.Equal(
		t,
		[]int{1, 2, 3, 4, 5, 6, 7, 8, 9},
		s,
	)
}
