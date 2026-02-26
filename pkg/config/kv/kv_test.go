package kv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecode(t *testing.T) {
	testCases := []struct {
		desc     string
		rootName string
		pairs    map[string]string
		expected *sample
	}{
		{
			desc:     "simple case",
			rootName: "ingress",
			pairs: map[string]string{
				"ingress/fielda":        "bar",
				"ingress/fieldb":        "1",
				"ingress/fieldc":        "true",
				"ingress/fieldd/0":      "one",
				"ingress/fieldd/1":      "two",
				"ingress/fielde":        "",
				"ingress/fieldf/Test1":  "A",
				"ingress/fieldf/Test2":  "B",
				"ingress/fieldg/0/name": "A",
				"ingress/fieldg/1/name": "B",
				"ingress/fieldh/":       "foo",
			},
			expected: &sample{
				FieldA: "bar",
				FieldB: 1,
				FieldC: true,
				FieldD: []string{"one", "two"},
				FieldE: &struct {
					Name string
				}{},
				FieldF: map[string]string{
					"Test1": "A",
					"Test2": "B",
				},
				FieldG: []sub{
					{Name: "A"},
					{Name: "B"},
				},
				FieldH: "foo",
			},
		},
		{
			desc:     "multi-level root name",
			rootName: "foo/bar/traefik",
			pairs: map[string]string{
				"foo/bar/traefik/fielda":        "bar",
				"foo/bar/traefik/fieldb":        "2",
				"foo/bar/traefik/fieldc":        "true",
				"foo/bar/traefik/fieldd/0":      "one",
				"foo/bar/traefik/fieldd/1":      "two",
				"foo/bar/traefik/fielde":        "",
				"foo/bar/traefik/fieldf/Test1":  "A",
				"foo/bar/traefik/fieldf/Test2":  "B",
				"foo/bar/traefik/fieldg/0/name": "A",
				"foo/bar/traefik/fieldg/1/name": "B",
				"foo/bar/traefik/fieldh/":       "foo",
			},
			expected: &sample{
				FieldA: "bar",
				FieldB: 2,
				FieldC: true,
				FieldD: []string{"one", "two"},
				FieldE: &struct {
					Name string
				}{},
				FieldF: map[string]string{
					"Test1": "A",
					"Test2": "B",
				},
				FieldG: []sub{
					{Name: "A"},
					{Name: "B"},
				},
				FieldH: "foo",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			element := &sample{}

			err := Decode(mapToPairs(test.pairs), element, test.rootName)
			require.NoError(t, err)

			assert.Equal(t, test.expected, element)
		})
	}
}

type sample struct {
	FieldA string
	FieldB int
	FieldC bool
	FieldD []string
	FieldE *struct {
		Name string
	} `kv:"allowEmpty"`
	FieldF map[string]string
	FieldG []sub
	FieldH string
}

type sub struct {
	Name string
}
