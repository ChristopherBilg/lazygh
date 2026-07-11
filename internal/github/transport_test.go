package github

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

// fakeRoundTripper returns a canned response/error.
type fakeRoundTripper struct {
	resp *http.Response
	err  error
}

func (f fakeRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return f.resp, f.err
}

func bufLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestLoggingTransportLogsRequestAndRateLimit(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("{}")),
	}
	resp.Header.Set("X-RateLimit-Remaining", "58")
	resp.Header.Set("X-RateLimit-Limit", "60")
	resp.Header.Set("X-RateLimit-Reset", "1720600000")

	var buf bytes.Buffer
	tr := loggingTransport{base: fakeRoundTripper{resp: resp}, logger: bufLogger(&buf)}

	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/user/repos", nil)
	got, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	if got != resp {
		t.Fatal("RoundTrip did not return the base response")
	}

	out := buf.String()
	for _, want := range []string{
		"github request", "status=200", "path=/user/repos",
		"github rate limit", "remaining=58", "limit=60", "reset=1720600000",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %q; got:\n%s", want, out)
		}
	}
}

func TestLoggingTransportLogsFailure(t *testing.T) {
	var buf bytes.Buffer
	wantErr := errors.New("dial tcp: boom")
	tr := loggingTransport{base: fakeRoundTripper{err: wantErr}, logger: bufLogger(&buf)}

	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/user/repos", nil)
	if _, err := tr.RoundTrip(req); !errors.Is(err, wantErr) {
		t.Fatalf("RoundTrip err = %v, want %v", err, wantErr)
	}
	if out := buf.String(); !strings.Contains(out, "github request failed") {
		t.Fatalf("log missing failure line; got:\n%s", out)
	}
}
