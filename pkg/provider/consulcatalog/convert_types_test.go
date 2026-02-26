package consulcatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_tagsToNeutralLabels(t *testing.T) {
	testCases := []struct {
		desc     string
		tags     []string
		prefix   string
		expected map[string]string
	}{
		{
			desc:     "without tags",
			expected: nil,
		},
		{
			desc:   "with a prefix",
			prefix: "test",
			tags: []string{
				"test.aaa=01",
				"test.bbb=02",
				"ccc=03",
				"test.ddd=04=to",
			},
			expected: map[string]string{
				"ingress.aaa": "01",
				"ingress.bbb": "02",
				"ingress.ddd": "04=to",
			},
		},

		{
			desc:   "with an empty prefix",
			prefix: "",
			tags: []string{
				"test.aaa=01",
				"test.bbb=02",
				"ccc=03",
				"test.ddd=04=to",
			},
			expected: map[string]string{
				"ingress.test.aaa": "01",
				"ingress.test.bbb": "02",
				"ingress.ccc":      "03",
				"ingress.test.ddd": "04=to",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			labels := tagsToNeutralLabels(test.tags, test.prefix)

			assert.Equal(t, test.expected, labels)
		})
	}
}
