package github

import (
	"bytes"
	"context"
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
	if got, want := reposEndpoint(50), "user/repos?sort=pushed&per_page=50"; got != want {
		t.Errorf("reposEndpoint(50) = %q, want %q", got, want)
	}
	if got, want := reposEndpoint(10), "user/repos?sort=pushed&per_page=10"; got != want {
		t.Errorf("reposEndpoint(10) = %q, want %q", got, want)
	}
}

func TestPRCommentsEndpoint(t *testing.T) {
	t.Parallel()
	if got, want := prCommentsEndpoint("octocat", "hello", 42), "repos/octocat/hello/issues/42/comments?per_page=100"; got != want {
		t.Errorf("prCommentsEndpoint = %q, want %q", got, want)
	}
}
