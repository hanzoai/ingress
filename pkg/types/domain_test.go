package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDomain_ToStrArray(t *testing.T) {
	testCases := []struct {
		desc     string
		domain   Domain
		expected []string
	}{
		{
			desc: "with Main and SANs",
			domain: Domain{
				Main: "foo.com",
				SANs: []string{"bar.foo.com", "bir.foo.com"},
			},
			expected: []string{"foo.com", "bar.foo.com", "bir.foo.com"},
		},
		{
			desc: "without SANs",
			domain: Domain{
				Main: "foo.com",
			},
			expected: []string{"foo.com"},
		},
		{
			desc: "without Main",
			domain: Domain{
				SANs: []string{"bar.foo.com", "bir.foo.com"},
			},
			expected: []string{"bar.foo.com", "bir.foo.com"},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			domains := test.domain.ToStrArray()
			assert.Equal(t, test.expected, domains)
		})
	}
}

func TestDomain_Set(t *testing.T) {
	testCases := []struct {
		desc       string
		rawDomains []string
		expected   Domain
	}{
		{
			desc:       "with 3 domains",
			rawDomains: []string{"foo.com", "bar.foo.com", "bir.foo.com"},
			expected: Domain{
				Main: "foo.com",
				SANs: []string{"bar.foo.com", "bir.foo.com"},
			},
		},
		{
			desc:       "with 1 domain",
			rawDomains: []string{"foo.com"},
			expected: Domain{
				Main: "foo.com",
				SANs: []string{},
			},
		},
		{
			desc:       "",
			rawDomains: nil,
			expected:   Domain{},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			domain := Domain{}
			domain.Set(test.rawDomains)

			assert.Equal(t, test.expected, domain)
		})
	}
}

func TestMatchDomain(t *testing.T) {
	testCases := []struct {
		desc       string
		certDomain string
		domain     string
		expected   bool
	}{
		{
			desc:       "exact match",
			certDomain: "ingress.wtf",
			domain:     "ingress.wtf",
			expected:   true,
		},
		{
			desc:       "wildcard and root domain",
			certDomain: "*.ingress.test",
			domain:     "ingress.wtf",
			expected:   false,
		},
		{
			desc:       "wildcard and sub domain",
			certDomain: "*.ingress.test",
			domain:     "sub.ingress.test",
			expected:   true,
		},
		{
			desc:       "wildcard and sub sub domain",
			certDomain: "*.ingress.test",
			domain:     "sub.sub.ingress.test",
			expected:   false,
		},
		{
			desc:       "double wildcard and sub sub domain",
			certDomain: "*.*.ingress.test",
			domain:     "sub.sub.ingress.test",
			expected:   true,
		},
		{
			desc:       "sub sub domain and invalid wildcard",
			certDomain: "sub.*.ingress.test",
			domain:     "sub.sub.ingress.test",
			expected:   false,
		},
		{
			desc:       "sub sub domain and valid wildcard",
			certDomain: "*.sub.ingress.test",
			domain:     "sub.sub.ingress.test",
			expected:   true,
		},
		{
			desc:       "dot replaced by a char",
			certDomain: "sub.sub.ingress.test",
			domain:     "sub.sub.ingressitest",
			expected:   false,
		},
		{
			desc:       "*",
			certDomain: "*",
			domain:     "sub.sub.ingress.test",
			expected:   false,
		},
		{
			desc:       "?",
			certDomain: "?",
			domain:     "sub.sub.ingress.test",
			expected:   false,
		},
		{
			desc:       "...................",
			certDomain: "...................",
			domain:     "sub.sub.ingress.test",
			expected:   false,
		},
		{
			desc:       "wildcard and *",
			certDomain: "*.ingress.test",
			domain:     "*.*.ingress.test",
			expected:   false,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			domains := MatchDomain(test.domain, test.certDomain)
			assert.Equal(t, test.expected, domains)
		})
	}
}
