package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/repository"
)

func TestNewClientSetsFields(t *testing.T) {
	t.Parallel()
	c := NewClient(ClientConfig{RESTTimeout: 5 * time.Second, SubprocessTimeout: time.Minute, RepoPageSize: 25})
	if c.restTimeout != 5*time.Second {
		t.Errorf("restTimeout = %v, want 5s", c.restTimeout)
	}
	if c.subprocessTimeout != time.Minute {
		t.Errorf("subprocessTimeout = %v, want 1m", c.subprocessTimeout)
	}
	if c.pageSize != 25 {
		t.Errorf("pageSize = %d, want 25", c.pageSize)
	}
}

func TestNewClientAppliesDefaults(t *testing.T) {
	t.Parallel()
	c := NewClient(ClientConfig{})
	if c.restTimeout != defaultRESTTimeout {
		t.Errorf("restTimeout = %v, want %v", c.restTimeout, defaultRESTTimeout)
	}
	if c.subprocessTimeout != defaultSubprocessTimeout {
		t.Errorf("subprocessTimeout = %v, want %v", c.subprocessTimeout, defaultSubprocessTimeout)
	}
	if c.pageSize != defaultRepoPageSize {
		t.Errorf("pageSize = %d, want %d", c.pageSize, defaultRepoPageSize)
	}
}

func TestRESTClientOptionsSetsTimeout(t *testing.T) {
	t.Parallel()
	c := NewClient(ClientConfig{})
	if got := c.restClientOptions().Timeout; got != defaultRESTTimeout {
		t.Fatalf("options Timeout = %v, want %v", got, defaultRESTTimeout)
	}
}

func TestNewRESTClientWrapsInitError(t *testing.T) {
	// No t.Parallel: t.Setenv mutates process environment.
	// Hermetic no-auth env: clear every token source and point config at an
	// empty dir so go-gh resolves no token and NewRESTClient fails.
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")
	t.Setenv("GITHUB_ENTERPRISE_TOKEN", "")
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	// go-gh also falls back to `gh auth token --secure-storage` (the system
	// keyring); point GH_PATH at a nonexistent binary so that fallback fails too.
	t.Setenv("GH_PATH", filepath.Join(t.TempDir(), "no-such-gh"))

	_, err := NewClient(ClientConfig{}).newRESTClient()
	if err == nil {
		t.Fatal("expected newRESTClient to fail with no auth token available")
	}
	if !errors.Is(err, ErrClientInit) {
		t.Fatalf("error %v does not match ErrClientInit", err)
	}
}

func TestCheckoutPRAppliesDeadline(t *testing.T) {
	t.Parallel()
	var deadline time.Time
	var hasDeadline bool
	var gotArgs []string
	c := &Client{
		subprocessTimeout: 30 * time.Second,
		currentRepo: func() (repository.Repository, error) {
			return repository.Repository{Owner: "octocat", Name: "hello"}, nil
		},
		exec: func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
			deadline, hasDeadline = ctx.Deadline()
			gotArgs = args
			return bytes.Buffer{}, bytes.Buffer{}, nil
		},
	}

	if err := c.CheckoutPR(t.Context(), "octocat", "hello", 7); err != nil {
		t.Fatalf("CheckoutPR returned error: %v", err)
	}
	if !hasDeadline {
		t.Fatal("expected CheckoutPR to pass a context with a deadline")
	}
	wantWindow := 30 * time.Second
	if d := time.Until(deadline); d < wantWindow-2*time.Second || d > wantWindow+time.Second {
		t.Fatalf("deadline in %v, want ~%v", d, wantWindow)
	}
	if wantArgs := []string{"pr", "checkout", "7", "--repo", "octocat/hello"}; !slices.Equal(gotArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestOpenPRInBrowserAppliesDeadline(t *testing.T) {
	t.Parallel()
	var deadline time.Time
	var hasDeadline bool
	var gotArgs []string
	c := &Client{
		subprocessTimeout: 30 * time.Second,
		exec: func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
			deadline, hasDeadline = ctx.Deadline()
			gotArgs = args
			return bytes.Buffer{}, bytes.Buffer{}, nil
		},
	}

	if err := c.OpenPRInBrowser(t.Context(), "octocat", "hello", 9); err != nil {
		t.Fatalf("OpenPRInBrowser returned error: %v", err)
	}
	if !hasDeadline {
		t.Fatal("expected OpenPRInBrowser to pass a context with a deadline")
	}
	wantWindow := 30 * time.Second
	if d := time.Until(deadline); d < wantWindow-2*time.Second || d > wantWindow+time.Second {
		t.Fatalf("deadline in %v, want ~%v", d, wantWindow)
	}
	if wantArgs := []string{"pr", "view", "9", "--repo", "octocat/hello", "--web"}; !slices.Equal(gotArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestRESTClientOptionsEnablesTUISafeLogging(t *testing.T) {
	t.Parallel()
	opts := NewClient(ClientConfig{}).restClientOptions()
	if !opts.LogIgnoreEnv {
		t.Error("LogIgnoreEnv = false, want true (GH_DEBUG must not write to stderr)")
	}
	if _, ok := opts.Transport.(loggingTransport); !ok {
		t.Errorf("Transport is %T, want loggingTransport", opts.Transport)
	}
}

func TestCheckoutPRLogsSuccessAndFailure(t *testing.T) {
	// No t.Parallel: slog.SetDefault mutates the process-wide default logger.
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	c := &Client{
		subprocessTimeout: 30 * time.Second,
		currentRepo: func() (repository.Repository, error) {
			return repository.Repository{Owner: "octocat", Name: "hello"}, nil
		},
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			return bytes.Buffer{}, bytes.Buffer{}, nil
		},
	}
	if err := c.CheckoutPR(t.Context(), "octocat", "hello", 7); err != nil {
		t.Fatalf("CheckoutPR success returned error: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "level=INFO") || !strings.Contains(out, "checked out pr") || !strings.Contains(out, "pr=7") {
		t.Fatalf("success log missing; got: %s", out)
	}

	buf.Reset()
	c.exec = func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
		return bytes.Buffer{}, bytes.Buffer{}, errors.New("boom")
	}
	if err := c.CheckoutPR(t.Context(), "octocat", "hello", 7); err == nil {
		t.Fatal("expected CheckoutPR to return the exec error")
	}
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "checkout failed") || !strings.Contains(out, "boom") {
		t.Fatalf("failure log missing; got: %s", out)
	}
}

func TestOpenPRInBrowserLogsSuccessAndFailure(t *testing.T) {
	// No t.Parallel: slog.SetDefault mutates the process-wide default logger.
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	c := &Client{
		subprocessTimeout: 30 * time.Second,
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			return bytes.Buffer{}, bytes.Buffer{}, nil
		},
	}
	if err := c.OpenPRInBrowser(t.Context(), "octocat", "hello", 9); err != nil {
		t.Fatalf("OpenPRInBrowser success returned error: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "level=INFO") || !strings.Contains(out, "opened pr in browser") || !strings.Contains(out, "pr=9") {
		t.Fatalf("success log missing; got: %s", out)
	}

	buf.Reset()
	c.exec = func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
		return bytes.Buffer{}, bytes.Buffer{}, errors.New("boom")
	}
	if err := c.OpenPRInBrowser(t.Context(), "octocat", "hello", 9); err == nil {
		t.Fatal("expected OpenPRInBrowser to return the exec error")
	}
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "open in browser failed") || !strings.Contains(out, "boom") {
		t.Fatalf("failure log missing; got: %s", out)
	}
}

func TestCheckoutPRRefusesNonLocalRepo(t *testing.T) {
	// No t.Parallel: slog.SetDefault mutates the process-wide default logger.
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	called := false
	c := &Client{
		currentRepo: func() (repository.Repository, error) {
			return repository.Repository{Owner: "someone", Name: "other"}, nil
		},
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			called = true
			return bytes.Buffer{}, bytes.Buffer{}, nil
		},
	}

	err := c.CheckoutPR(t.Context(), "octocat", "hello", 7)
	if !errors.Is(err, ErrNotLocalRepo) {
		t.Fatalf("err = %v, want ErrNotLocalRepo", err)
	}
	if called {
		t.Fatal("expected gh not to run when the selected repo is not lazygh's local repo")
	}
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "checkout unavailable") {
		t.Fatalf("expected a WARN 'checkout unavailable' log; got: %s", out)
	}
}

func TestCheckoutPRRefusesWhenLocalRepoUnknown(t *testing.T) {
	// No t.Parallel: slog.SetDefault mutates the process-wide default logger.
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	called := false
	c := &Client{
		currentRepo: func() (repository.Repository, error) {
			return repository.Repository{}, errors.New("no git remotes configured")
		},
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			called = true
			return bytes.Buffer{}, bytes.Buffer{}, nil
		},
	}

	err := c.CheckoutPR(t.Context(), "octocat", "hello", 7)
	if !errors.Is(err, ErrNotLocalRepo) {
		t.Fatalf("err = %v, want ErrNotLocalRepo", err)
	}
	if called {
		t.Fatal("expected gh not to run when the local repo cannot be resolved")
	}
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "cannot resolve local repository") {
		t.Fatalf("expected a WARN 'cannot resolve local repository' log; got: %s", out)
	}
}

func TestCheckoutPRMatchesCaseInsensitively(t *testing.T) {
	t.Parallel()
	var gotArgs []string
	c := &Client{
		subprocessTimeout: 30 * time.Second,
		currentRepo: func() (repository.Repository, error) {
			return repository.Repository{Owner: "OctoCat", Name: "Hello"}, nil
		},
		exec: func(_ context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
			gotArgs = args
			return bytes.Buffer{}, bytes.Buffer{}, nil
		},
	}

	if err := c.CheckoutPR(t.Context(), "octocat", "hello", 7); err != nil {
		t.Fatalf("CheckoutPR returned error: %v", err)
	}
	if want := []string{"pr", "checkout", "7", "--repo", "octocat/hello"}; !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}

func TestReposEndpoint(t *testing.T) {
	t.Parallel()
	if got, want := reposEndpoint(50, 1), "user/repos?sort=pushed&per_page=50&page=1"; got != want {
		t.Errorf("reposEndpoint(50, 1) = %q, want %q", got, want)
	}
	if got, want := reposEndpoint(10, 3), "user/repos?sort=pushed&per_page=10&page=3"; got != want {
		t.Errorf("reposEndpoint(10, 3) = %q, want %q", got, want)
	}
}

func TestPRsEndpoint(t *testing.T) {
	t.Parallel()
	if got, want := prsEndpoint("octocat", "hello", 100, 2), "repos/octocat/hello/pulls?per_page=100&page=2"; got != want {
		t.Errorf("prsEndpoint = %q, want %q", got, want)
	}
}

func TestPRCommentsEndpoint(t *testing.T) {
	t.Parallel()
	if got, want := prCommentsEndpoint("octocat", "hello", 42, 100, 1), "repos/octocat/hello/issues/42/comments?per_page=100&page=1"; got != want {
		t.Errorf("prCommentsEndpoint = %q, want %q", got, want)
	}
}

func TestPullRequestDecodesAuthorAndReviewers(t *testing.T) {
	t.Parallel()
	const data = `{"number":1,"title":"T","state":"open",` +
		`"user":{"login":"octocat"},` +
		`"requested_reviewers":[{"login":"hubot"},{"login":"octo-dev"}]}`
	var pr PullRequest
	if err := json.Unmarshal([]byte(data), &pr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pr.User.Login != "octocat" {
		t.Errorf("author = %q, want octocat", pr.User.Login)
	}
	if len(pr.RequestedReviewers) != 2 ||
		pr.RequestedReviewers[0].Login != "hubot" ||
		pr.RequestedReviewers[1].Login != "octo-dev" {
		t.Errorf("reviewers = %+v, want [hubot octo-dev]", pr.RequestedReviewers)
	}
}

func TestCurrentUserReturnsLogin(t *testing.T) {
	t.Parallel()
	c := &Client{cache: NewCache(), fetchLogin: func(context.Context) (string, error) {
		return "octocat", nil
	}}
	got, err := c.CurrentUser(t.Context())
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if got != "octocat" {
		t.Errorf("login = %q, want octocat", got)
	}
}

func TestCurrentUserMemoizes(t *testing.T) {
	t.Parallel()
	calls := 0
	c := &Client{cache: NewCache(), fetchLogin: func(context.Context) (string, error) {
		calls++
		return "octocat", nil
	}}
	for range 3 {
		if _, err := c.CurrentUser(t.Context()); err != nil {
			t.Fatalf("CurrentUser: %v", err)
		}
	}
	if calls != 1 {
		t.Errorf("fetchLogin called %d times, want 1 (memoized)", calls)
	}
}

func TestCurrentUserDoesNotCacheError(t *testing.T) {
	t.Parallel()
	calls := 0
	c := &Client{cache: NewCache(), fetchLogin: func(context.Context) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("boom")
		}
		return "octocat", nil
	}}
	if _, err := c.CurrentUser(t.Context()); err == nil {
		t.Fatal("expected first CurrentUser to error")
	}
	got, err := c.CurrentUser(t.Context())
	if err != nil {
		t.Fatalf("second CurrentUser: %v", err)
	}
	if got != "octocat" {
		t.Errorf("login = %q, want octocat after retry", got)
	}
	if calls != 2 {
		t.Errorf("fetchLogin called %d times, want 2 (error not cached)", calls)
	}
}

func TestNewClientMergeMethod(t *testing.T) {
	t.Parallel()
	if got := NewClient(ClientConfig{}).mergeMethod; got != "merge" {
		t.Errorf("default mergeMethod = %q, want merge", got)
	}
	if got := NewClient(ClientConfig{MergeMethod: "squash"}).mergeMethod; got != "squash" {
		t.Errorf("mergeMethod = %q, want squash", got)
	}
	if got := NewClient(ClientConfig{MergeMethod: "bogus"}).mergeMethod; got != "merge" {
		t.Errorf("invalid mergeMethod = %q, want merge (defaulted)", got)
	}
}

func TestApprovePRArgs(t *testing.T) {
	t.Parallel()
	var gotArgs []string
	c := &Client{subprocessTimeout: 30 * time.Second,
		exec: func(_ context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
			gotArgs = args
			return bytes.Buffer{}, bytes.Buffer{}, nil
		}}
	if err := c.ApprovePR(t.Context(), "octocat", "hello", 7); err != nil {
		t.Fatalf("ApprovePR: %v", err)
	}
	if want := []string{"pr", "review", "7", "--repo", "octocat/hello", "--approve"}; !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}

func TestMergePRUsesConfiguredMethod(t *testing.T) {
	t.Parallel()
	var gotArgs []string
	c := &Client{subprocessTimeout: 30 * time.Second, mergeMethod: "squash",
		exec: func(_ context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
			gotArgs = args
			return bytes.Buffer{}, bytes.Buffer{}, nil
		}}
	if err := c.MergePR(t.Context(), "octocat", "hello", 8); err != nil {
		t.Fatalf("MergePR: %v", err)
	}
	if want := []string{"pr", "merge", "8", "--repo", "octocat/hello", "--squash"}; !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}

func TestClosePRArgs(t *testing.T) {
	t.Parallel()
	var gotArgs []string
	c := &Client{subprocessTimeout: 30 * time.Second,
		exec: func(_ context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
			gotArgs = args
			return bytes.Buffer{}, bytes.Buffer{}, nil
		}}
	if err := c.ClosePR(t.Context(), "octocat", "hello", 9); err != nil {
		t.Fatalf("ClosePR: %v", err)
	}
	if want := []string{"pr", "close", "9", "--repo", "octocat/hello"}; !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}

func TestMergePRFoldsStderrIntoError(t *testing.T) {
	t.Parallel()
	c := &Client{subprocessTimeout: 30 * time.Second, mergeMethod: "merge",
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			stderr.WriteString("Pull request is not mergeable\n")
			return bytes.Buffer{}, stderr, errors.New("exit status 1")
		}}
	err := c.MergePR(t.Context(), "octocat", "hello", 8)
	if err == nil {
		t.Fatal("expected MergePR to return an error")
	}
	if !strings.Contains(err.Error(), "Pull request is not mergeable") {
		t.Fatalf("error %q should include gh's stderr reason", err)
	}
}

func TestApprovePRLogsSuccessAndFailure(t *testing.T) {
	// No t.Parallel: slog.SetDefault mutates the process-wide default logger.
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	c := &Client{subprocessTimeout: 30 * time.Second,
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			return bytes.Buffer{}, bytes.Buffer{}, nil
		}}
	if err := c.ApprovePR(t.Context(), "octocat", "hello", 7); err != nil {
		t.Fatalf("ApprovePR success returned error: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "level=INFO") || !strings.Contains(out, "approved pr") || !strings.Contains(out, "pr=7") {
		t.Fatalf("success log missing; got: %s", out)
	}

	buf.Reset()
	c.exec = func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
		return bytes.Buffer{}, bytes.Buffer{}, errors.New("boom")
	}
	if err := c.ApprovePR(t.Context(), "octocat", "hello", 7); err == nil {
		t.Fatal("expected ApprovePR to return the exec error")
	}
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "approve pr failed") || !strings.Contains(out, "boom") {
		t.Fatalf("failure log missing; got: %s", out)
	}
}

func TestClosePRLogsSuccessAndFailure(t *testing.T) {
	// No t.Parallel: slog.SetDefault mutates the process-wide default logger.
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	c := &Client{subprocessTimeout: 30 * time.Second,
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			return bytes.Buffer{}, bytes.Buffer{}, nil
		}}
	if err := c.ClosePR(t.Context(), "octocat", "hello", 9); err != nil {
		t.Fatalf("ClosePR success returned error: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "level=INFO") || !strings.Contains(out, "closed pr") || !strings.Contains(out, "pr=9") {
		t.Fatalf("success log missing; got: %s", out)
	}

	buf.Reset()
	c.exec = func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
		return bytes.Buffer{}, bytes.Buffer{}, errors.New("boom")
	}
	if err := c.ClosePR(t.Context(), "octocat", "hello", 9); err == nil {
		t.Fatal("expected ClosePR to return the exec error")
	}
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "close pr failed") || !strings.Contains(out, "boom") {
		t.Fatalf("failure log missing; got: %s", out)
	}
}

func TestApprovePRAppliesDeadline(t *testing.T) {
	t.Parallel()
	var deadline time.Time
	var hasDeadline bool
	c := &Client{subprocessTimeout: 30 * time.Second,
		exec: func(ctx context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			deadline, hasDeadline = ctx.Deadline()
			return bytes.Buffer{}, bytes.Buffer{}, nil
		}}
	if err := c.ApprovePR(t.Context(), "octocat", "hello", 7); err != nil {
		t.Fatalf("ApprovePR returned error: %v", err)
	}
	if !hasDeadline {
		t.Fatal("expected ApprovePR to pass a context with a deadline")
	}
	wantWindow := 30 * time.Second
	if d := time.Until(deadline); d < wantWindow-2*time.Second || d > wantWindow+time.Second {
		t.Fatalf("deadline in %v, want ~%v", d, wantWindow)
	}
}

func TestPRSubcommandEmptyStderrReturnsRawError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("network unreachable")
	c := &Client{subprocessTimeout: 30 * time.Second,
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			return bytes.Buffer{}, bytes.Buffer{}, sentinel
		}}
	err := c.ClosePR(t.Context(), "octocat", "hello", 9)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want it to return the sentinel when stderr is empty", err)
	}
	if err.Error() != "network unreachable" {
		t.Fatalf("err = %q, want the raw error with no stderr suffix when stderr is empty", err.Error())
	}
}

func TestFetchPRDiffAppliesDeadlineAndArgs(t *testing.T) {
	t.Parallel()
	var deadline time.Time
	var hasDeadline bool
	var gotArgs []string
	c := &Client{
		subprocessTimeout: 30 * time.Second,
		exec: func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
			deadline, hasDeadline = ctx.Deadline()
			gotArgs = args
			stdout.WriteString("diff --git a/x b/x\n")
			return stdout, stderr, nil
		},
	}
	got, err := c.FetchPRDiff(t.Context(), "octocat", "hello", 9)
	if err != nil {
		t.Fatalf("FetchPRDiff returned error: %v", err)
	}
	if !hasDeadline {
		t.Fatal("expected FetchPRDiff to pass a context with a deadline")
	}
	wantWindow := 30 * time.Second
	if d := time.Until(deadline); d < wantWindow-2*time.Second || d > wantWindow+time.Second {
		t.Fatalf("deadline in %v, want ~%v", d, wantWindow)
	}
	if wantArgs := []string{"pr", "diff", "9", "--repo", "octocat/hello"}; !slices.Equal(gotArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", gotArgs, wantArgs)
	}
	if !strings.Contains(got, "diff --git") {
		t.Fatalf("diff = %q, want it to contain the stdout", got)
	}
}

func TestFetchPRDiffLogsFailureWithStderr(t *testing.T) {
	// No t.Parallel: slog.SetDefault mutates the process-wide default logger.
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	c := &Client{
		subprocessTimeout: 30 * time.Second,
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			stderr.WriteString("no pull requests found")
			return stdout, stderr, errors.New("exit status 1")
		},
	}
	_, err := c.FetchPRDiff(t.Context(), "octocat", "hello", 9)
	if err == nil {
		t.Fatal("expected FetchPRDiff to return the exec error")
	}
	if !strings.Contains(err.Error(), "no pull requests found") {
		t.Fatalf("returned error should fold in the stderr detail; got: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "fetch pr diff failed") || !strings.Contains(out, "no pull requests found") {
		t.Fatalf("failure log missing stderr; got: %s", out)
	}
}

func TestFetchPRDiffLogsSuccess(t *testing.T) {
	// No t.Parallel: slog.SetDefault mutates the process-wide default logger.
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	c := &Client{
		subprocessTimeout: 30 * time.Second,
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			stdout.WriteString("diff --git a/x b/x\n")
			return stdout, stderr, nil
		},
	}
	if _, err := c.FetchPRDiff(t.Context(), "octocat", "hello", 9); err != nil {
		t.Fatalf("FetchPRDiff: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "level=INFO") || !strings.Contains(out, "fetched pr diff") || !strings.Contains(out, "pr=9") {
		t.Fatalf("success log missing; got: %s", out)
	}
}

func TestPRDiffMemoizes(t *testing.T) {
	t.Parallel()
	calls := 0
	c := &Client{
		cache:             NewCache(),
		subprocessTimeout: 30 * time.Second,
		exec: func(_ context.Context, _ ...string) (stdout, stderr bytes.Buffer, err error) {
			calls++
			stdout.WriteString("diff --git a/x b/x\n")
			return stdout, stderr, nil
		},
	}
	for range 3 {
		if _, err := c.PRDiff(t.Context(), "octocat", "hello", 9, false); err != nil {
			t.Fatalf("PRDiff: %v", err)
		}
	}
	if calls != 1 {
		t.Errorf("exec called %d times, want 1 (memoized)", calls)
	}
	if _, err := c.PRDiff(t.Context(), "octocat", "hello", 9, true); err != nil {
		t.Fatalf("PRDiff force: %v", err)
	}
	if calls != 2 {
		t.Errorf("exec called %d times after force, want 2 (force bypasses cache)", calls)
	}
}

func TestPaginateStopsOnShortPage(t *testing.T) {
	t.Parallel()
	// per_page=3: a full first page (3 items) then a short page (2) stops paging.
	pages := [][]int{{1, 2, 3}, {4, 5}}
	calls := 0
	got, err := paginate(t.Context(), 3, func(_ context.Context, page int) ([]int, error) {
		calls++
		return pages[page-1], nil
	})
	if err != nil {
		t.Fatalf("paginate: %v", err)
	}
	if want := []int{1, 2, 3, 4, 5}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestPaginateExactMultipleStopsOnEmptyPage(t *testing.T) {
	t.Parallel()
	// per_page=2 with an exact-multiple total: two full pages then an empty page
	// (len 0 < 2) stops paging — one extra request, as documented.
	pages := [][]int{{1, 2}, {3, 4}, {}}
	calls := 0
	got, err := paginate(t.Context(), 2, func(_ context.Context, page int) ([]int, error) {
		calls++
		return pages[page-1], nil
	})
	if err != nil {
		t.Fatalf("paginate: %v", err)
	}
	if want := []int{1, 2, 3, 4}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (one extra empty request)", calls)
	}
}

func TestPaginateSingleShortPage(t *testing.T) {
	t.Parallel()
	calls := 0
	got, err := paginate(t.Context(), 100, func(_ context.Context, _ int) ([]int, error) {
		calls++
		return []int{1, 2}, nil
	})
	if err != nil {
		t.Fatalf("paginate: %v", err)
	}
	if want := []int{1, 2}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestPaginateReturnsErrorFromFirstPage(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("boom")
	_, err := paginate(t.Context(), 10, func(_ context.Context, _ int) ([]int, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestPaginateReturnsErrorFromLaterPage(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("boom on page 2")
	calls := 0
	got, err := paginate(t.Context(), 2, func(_ context.Context, page int) ([]int, error) {
		calls++
		if page == 1 {
			return []int{1, 2}, nil // full page => keep paging
		}
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatalf("expected nil result on later-page error (no partial), got %v", got)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestPaginateHitsMaxPagesCapAndWarns(t *testing.T) {
	// No t.Parallel: slog.SetDefault mutates the process-wide default logger.
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	calls := 0
	// per_page=1 with every page full (1 item) never short-circuits, so paging runs
	// until the maxPages cap.
	got, err := paginate(t.Context(), 1, func(_ context.Context, _ int) ([]int, error) {
		calls++
		return []int{0}, nil
	})
	if err != nil {
		t.Fatalf("paginate: %v", err)
	}
	if calls != maxPages {
		t.Fatalf("calls = %d, want %d (max-pages cap)", calls, maxPages)
	}
	if len(got) != maxPages {
		t.Fatalf("len(got) = %d, want %d", len(got), maxPages)
	}
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "max-pages cap") {
		t.Fatalf("expected a WARN 'max-pages cap' log; got: %s", out)
	}
}
