package github

import (
	"io"
	"log/slog"
	"os"
	"testing"
)

// TestMain silences slog output by default so package test runs stay clean;
// tests that assert on log output install their own logger.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}
