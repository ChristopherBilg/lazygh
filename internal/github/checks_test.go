package github

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBucketRollup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		items []rollupItem
		want  CheckStatus
	}{
		{"empty is none", nil, CheckNone},
		{"single success check run", []rollupItem{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"}}, CheckPassing},
		{"one failure among successes", []rollupItem{
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"},
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "FAILURE"},
		}, CheckFailing},
		{"in-progress among successes is pending", []rollupItem{
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"},
			{Typename: "CheckRun", Status: "IN_PROGRESS"},
		}, CheckPending},
		{"failure outranks pending, failure first", []rollupItem{
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "FAILURE"},
			{Typename: "CheckRun", Status: "IN_PROGRESS"},
		}, CheckFailing},
		{"failure outranks pending, pending first", []rollupItem{
			{Typename: "CheckRun", Status: "IN_PROGRESS"},
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "FAILURE"},
		}, CheckFailing},
		{"neutral and skipped are passing", []rollupItem{
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "NEUTRAL"},
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SKIPPED"},
		}, CheckPassing},
		{"cancelled is failing", []rollupItem{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "CANCELLED"}}, CheckFailing},
		{"timed out is failing", []rollupItem{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "TIMED_OUT"}}, CheckFailing},
		{"action required is failing", []rollupItem{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "ACTION_REQUIRED"}}, CheckFailing},
		{"unknown conclusion is passing", []rollupItem{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "MYSTERY"}}, CheckPassing},
		{"status context success is passing", []rollupItem{{Typename: "StatusContext", State: "SUCCESS"}}, CheckPassing},
		{"status context pending", []rollupItem{{Typename: "StatusContext", State: "PENDING"}}, CheckPending},
		{"status context expected is pending", []rollupItem{{Typename: "StatusContext", State: "EXPECTED"}}, CheckPending},
		{"status context failure", []rollupItem{{Typename: "StatusContext", State: "FAILURE"}}, CheckFailing},
		{"status context error is failing", []rollupItem{{Typename: "StatusContext", State: "ERROR"}}, CheckFailing},
		{"mixed check run and status context failure", []rollupItem{
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"},
			{Typename: "StatusContext", State: "FAILURE"},
		}, CheckFailing},
		{"startup failure is failing", []rollupItem{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "STARTUP_FAILURE"}}, CheckFailing},
		{"stale is failing", []rollupItem{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "STALE"}}, CheckFailing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := bucketRollup(tt.items); got != tt.want {
				t.Errorf("bucketRollup() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRepoPRChecksParsesAndBuckets(t *testing.T) {
	t.Parallel()
	var gotArgs []string
	var hasDeadline bool
	c := &Client{
		subprocessTimeout: 30 * time.Second,
		exec: func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
			gotArgs = args
			_, hasDeadline = ctx.Deadline()
			stdout.WriteString(`[
				{"number":1,"statusCheckRollup":[{"__typename":"CheckRun","status":"COMPLETED","conclusion":"SUCCESS"}]},
				{"number":2,"statusCheckRollup":[{"__typename":"CheckRun","status":"COMPLETED","conclusion":"FAILURE"}]},
				{"number":3,"statusCheckRollup":[{"__typename":"CheckRun","status":"IN_PROGRESS"}]},
				{"number":4,"statusCheckRollup":[]},
				{"number":5,"statusCheckRollup":null}
			]`)
			return stdout, stderr, nil
		},
	}
	got, err := c.RepoPRChecks(t.Context(), "octocat", "hello")
	if err != nil {
		t.Fatalf("RepoPRChecks error: %v", err)
	}
	want := map[int]CheckStatus{1: CheckPassing, 2: CheckFailing, 3: CheckPending, 4: CheckNone, 5: CheckNone}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
	}
	for n, ws := range want {
		if got[n] != ws {
			t.Errorf("PR #%d = %v, want %v", n, got[n], ws)
		}
	}
	if !hasDeadline {
		t.Error("expected a subprocess deadline on the context")
	}
	wantArgs := []string{"pr", "list", "--repo", "octocat/hello", "--state", "open", "--json", "number,statusCheckRollup", "--limit", "100"}
	if !slices.Equal(gotArgs, wantArgs) {
		t.Errorf("args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestRepoPRChecksFoldsStderr(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	c := &Client{
		subprocessTimeout: 30 * time.Second,
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			stderr.WriteString("could not resolve to a Repository\n")
			return stdout, stderr, sentinel
		},
	}
	_, err := c.RepoPRChecks(t.Context(), "octocat", "hello")
	if !errors.Is(err, sentinel) {
		t.Fatalf("error %v does not wrap sentinel", err)
	}
	if !strings.Contains(err.Error(), "could not resolve to a Repository") {
		t.Errorf("error %q missing stderr detail", err.Error())
	}
}

func TestRepoPRChecksParseError(t *testing.T) {
	t.Parallel()
	c := &Client{
		subprocessTimeout: 30 * time.Second,
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			stdout.WriteString("not json")
			return stdout, stderr, nil
		},
	}
	if _, err := c.RepoPRChecks(t.Context(), "octocat", "hello"); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestRepoPRChecksEmptyStderrReturnsRawError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	c := &Client{
		subprocessTimeout: 30 * time.Second,
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			return stdout, stderr, sentinel // stderr empty
		},
	}
	_, err := c.RepoPRChecks(t.Context(), "octocat", "hello")
	if !errors.Is(err, sentinel) {
		t.Fatalf("error %v does not wrap sentinel", err)
	}
}
