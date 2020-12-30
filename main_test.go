package main

import (
	"testing"

	assert "github.com/stretchr/testify/require"
)

func TestMergeTweets(t *testing.T) {
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

func TestSliceUniq(t *testing.T) {
	s := []int{1, 2, 2, 3, 4, 5, 6, 7, 7, 7, 8, 9}
	s = sliceUniq(s, func(i int) interface{} { return s[i] }).([]int)

	assert.Equal(
		t,
		[]int{1, 2, 3, 4, 5, 6, 7, 8, 9},
		s,
	)
}
