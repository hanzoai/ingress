package types

import (
	"bytes"
	"io"
	"net/http"

	"github.com/valyala/fasthttp"
	zaphttp "github.com/zap-proto/http"
)

// zapRoundTripper carries net/http requests over the ZAP wire.
//
// zaphttp.Transport is shaped like fasthttp — Do(req, resp) — while the
// OpenTelemetry exporters, and most of the ecosystem, hand you an *http.Client.
// This adapts one to the other so a client can be pointed at ZAP without the
// caller knowing the wire changed.
type zapRoundTripper struct {
	transport *zaphttp.Transport
}

// zapClient returns an *http.Client whose requests travel as Cap'n Proto frames
// over TCP to addr, rather than as HTTP/1.1 or gRPC.
func zapClient(addr string) *http.Client {
	return &http.Client{Transport: &zapRoundTripper{transport: zaphttp.NewTransport(addr)}}
}

func (rt *zapRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	freq := fasthttp.AcquireRequest()
	fresp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(freq)
	defer fasthttp.ReleaseResponse(fresp)

	freq.Header.SetMethod(req.Method)
	freq.SetRequestURI(req.URL.String())
	for name, values := range req.Header {
		for _, v := range values {
			freq.Header.Add(name, v)
		}
	}
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		freq.SetBody(body)
	}

	if err := rt.transport.Do(freq, fresp); err != nil {
		return nil, err
	}

	resp := &http.Response{
		StatusCode: fresp.StatusCode(),
		Header:     http.Header{},
		Request:    req,
	}
	fresp.Header.VisitAll(func(k, v []byte) {
		resp.Header.Add(string(k), string(v))
	})
	body := append([]byte(nil), fresp.Body()...)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))

	return resp, nil
}
