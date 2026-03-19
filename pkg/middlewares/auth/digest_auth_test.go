package auth

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/hanzoai/ingress/v3/pkg/config/dynamic"
	"github.com/hanzoai/ingress/v3/pkg/testhelpers"
)

func TestDigestAuthError(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ingress")
	})

	auth := dynamic.DigestAuth{
		Users: []string{"test"},
	}
	_, err := NewDigest(t.Context(), next, auth, "authName")
	assert.Error(t, err)
}

func TestDigestAuthFail(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ingress")
	})

	auth := dynamic.DigestAuth{
		Users: []string{"test:ingress:c6ab6673638f15e297094e67b4f57798"},
	}
	authMiddleware, err := NewDigest(t.Context(), next, auth, "authName")
	require.NoError(t, err)
	assert.NotNil(t, authMiddleware, "this should not be nil")

	ts := httptest.NewServer(authMiddleware)
	defer ts.Close()

	client := http.DefaultClient
	req := testhelpers.MustNewRequest(http.MethodGet, ts.URL, nil)
	req.SetBasicAuth("test", "test")

	res, err := client.Do(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestDigestAuthUsersFromFile(t *testing.T) {
	testCases := []struct {
		desc            string
		userFileContent string
		expectedUsers   map[string]string
		givenUsers      []string
		realm           string
	}{
		{
			desc:            "Finds the users in the file",
			userFileContent: "test:ingress:c6ab6673638f15e297094e67b4f57798\ntest2:ingress:4edd369c81fa595a914e837ee66e26c8\n",
			givenUsers:      []string{},
			expectedUsers:   map[string]string{"test": "test", "test2": "test2"},
		},
		{
			desc:            "Merges given users with users from the file",
			userFileContent: "test:ingress:c6ab6673638f15e297094e67b4f57798\n",
			givenUsers:      []string{"test2:ingress:4edd369c81fa595a914e837ee66e26c8", "test3:ingress:f8177798c56462b122a0758747d4ae1c"},
			expectedUsers:   map[string]string{"test": "test", "test2": "test2", "test3": "test3"},
		},
		{
			desc:            "Given users have priority over users in the file",
			userFileContent: "test:ingress:c6ab6673638f15e297094e67b4f57798\ntest2:ingress:4edd369c81fa595a914e837ee66e26c8\n",
			givenUsers:      []string{"test2:ingress:349c99e8d9187708c204c0c53ff91b5f"},
			expectedUsers:   map[string]string{"test": "test", "test2": "overridden"},
		},
		{
			desc:            "Should authenticate the correct user based on the realm",
			userFileContent: "test:ingress:c6ab6673638f15e297094e67b4f57798\ntest:ingresser:d870e4ff60baacee185e071815225f40\n",
			givenUsers:      []string{},
			expectedUsers:   map[string]string{"test": "test2"},
			realm:           "ingresser",
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			// Creates the temporary configuration file with the users
			usersFile, err := os.CreateTemp(t.TempDir(), "auth-users")
			require.NoError(t, err)

			_, err = usersFile.WriteString(test.userFileContent)
			require.NoError(t, err)

			// Creates the configuration for our Authenticator
			authenticatorConfiguration := dynamic.DigestAuth{
				Users:     test.givenUsers,
				UsersFile: usersFile.Name(),
				Realm:     test.realm,
			}

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintln(w, "ingress")
			})

			authenticator, err := NewDigest(t.Context(), next, authenticatorConfiguration, "authName")
			require.NoError(t, err)

			ts := httptest.NewServer(authenticator)
			defer ts.Close()

			for userName, userPwd := range test.expectedUsers {
				req := testhelpers.MustNewRequest(http.MethodGet, ts.URL, nil)
				digestRequest := newDigestRequest(userName, userPwd, http.DefaultClient)

				var res *http.Response
				res, err = digestRequest.Do(req)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, res.StatusCode, "Cannot authenticate user "+userName)

				var body []byte
				body, err = io.ReadAll(res.Body)
				require.NoError(t, err)
				err = res.Body.Close()
				require.NoError(t, err)

				require.Equal(t, "ingress\n", string(body))
			}

			// Checks that user foo doesn't work
			req := testhelpers.MustNewRequest(http.MethodGet, ts.URL, nil)
			digestRequest := newDigestRequest("foo", "foo", http.DefaultClient)

			var res *http.Response
			res, err = digestRequest.Do(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusUnauthorized, res.StatusCode)

			var body []byte
			body, err = io.ReadAll(res.Body)
			require.NoError(t, err)
			err = res.Body.Close()
			require.NoError(t, err)

			require.NotContains(t, "ingress", string(body))
		})
	}
}
