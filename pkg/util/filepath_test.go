package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_BaseName(t *testing.T) {
	assert := require.New(t)
	testCases := []struct {
		desc     string
		path     string
		expected string
	}{
		{
			desc:     "should return 'foo'",
			path:     "foo.txt",
			expected: "foo",
		},
		{
			desc:     "should return 'bar'",
			path:     "/path/to/bar.txt",
			expected: "bar",
		},
		{
			desc:     "should return 'baz'",
			path:     "baz",
			expected: "baz",
		},
	}
	for _, tc := range testCases {
		result := BaseName(tc.path)
		assert.Equal(result, tc.expected, tc.desc)
	}
}
