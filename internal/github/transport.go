package github

import (
	"log/slog"
	"net/http"
	"time"
)

// loggingTransport is an http.RoundTripper that records each GitHub API request
// and the rate-limit headers from its response, so diagnostics land in the log
// file instead of the terminal. It wraps a base RoundTripper (http.DefaultTransport
// in production). go-gh layers its own auth/header round-tripper ON TOP of this
// one, so requests already carry auth when RoundTrip runs.
type loggingTransport struct {
	base   http.RoundTripper
	logger *slog.Logger // nil => slog.Default(); set explicitly in tests
}

func (t loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	l := t.logger
	if l == nil {
		l = slog.Default()
	}

	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	dur := time.Since(start)

	if err != nil {
		l.Debug("github request failed",
			"method", req.Method, "path", req.URL.Path, "dur", dur, "err", err)
		return resp, err
	}

	l.Debug("github request",
		"method", req.Method, "path", req.URL.Path,
		"status", resp.StatusCode, "dur", dur)

	if resp.Header.Get("X-RateLimit-Limit") != "" {
		l.Info("github rate limit",
			"remaining", resp.Header.Get("X-RateLimit-Remaining"),
			"limit", resp.Header.Get("X-RateLimit-Limit"),
			"reset", resp.Header.Get("X-RateLimit-Reset"))
	}

	return resp, err
}
