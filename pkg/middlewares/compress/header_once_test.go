package compress

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusCounter records every WriteHeader the client-facing writer receives.
type statusCounter struct {
	*httptest.ResponseRecorder
	statuses []int
}

func (s *statusCounter) WriteHeader(code int) {
	s.statuses = append(s.statuses, code)
	s.ResponseRecorder.WriteHeader(code)
}

// A response the middleware decides not to compress sends its status once. The
// second send is what net/http logs as "superfluous response.WriteHeader call",
// once per such response — every streamed completion, under a config that
// compresses only text bodies.
func TestUncompressedResponseSendsStatusOnce(t *testing.T) {
	cases := []struct {
		desc string
		conf dynamic.Compress
		ct   string
		ce   string
	}{
		{desc: "not included", conf: dynamic.Compress{Encodings: []string{"zstd", "br"}, IncludedContentTypes: []string{"application/json"}}, ct: "text/event-stream"},
		{desc: "excluded", conf: dynamic.Compress{Encodings: []string{"zstd", "br"}, ExcludedContentTypes: []string{"text/event-stream"}}, ct: "text/event-stream"},
		{desc: "already encoded", conf: dynamic.Compress{Encodings: []string{"zstd", "br"}}, ct: "application/json", ce: "gzip"},
		{desc: "unparsable type", conf: dynamic.Compress{Encodings: []string{"zstd", "br"}}, ct: "not a type;;"},
	}
	for _, enc := range []string{"zstd", "br"} {
		for _, tc := range cases {
			t.Run(enc+"/"+tc.desc, func(t *testing.T) {
				h, err := New(context.Background(), http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
					rw.Header().Set("Content-Type", tc.ct)
					if tc.ce != "" {
						rw.Header().Set("Content-Encoding", tc.ce)
					}
					rw.WriteHeader(http.StatusCreated)
					_, _ = rw.Write(bigTestBody)
				}), tc.conf, "test")
				require.NoError(t, err)

				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set(acceptEncodingHeader, enc)
				rw := &statusCounter{ResponseRecorder: httptest.NewRecorder()}
				h.ServeHTTP(rw, req)

				assert.Equal(t, []int{http.StatusCreated}, rw.statuses)
				assert.Equal(t, tc.ce, rw.Header().Get(contentEncoding))
				assert.Equal(t, bigTestBody, rw.Body.Bytes())
			})
		}
	}
}
