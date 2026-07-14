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

func TestTimeoutValues(t *testing.T) {
	if RESTTimeout.Seconds() != 10 {
		t.Fatalf("RESTTimeout = %v, want 10s", RESTTimeout)
	}
	if SubprocessTimeout.Seconds() != 30 {
		t.Fatalf("SubprocessTimeout = %v, want 30s", SubprocessTimeout)
	}
}

func TestRESTClientOptionsSetsTimeout(t *testing.T) {
	if got := restClientOptions().Timeout; got != RESTTimeout {
		t.Fatalf("options Timeout = %v, want %v", got, RESTTimeout)
	}
}

func TestNewRESTClientWrapsInitError(t *testing.T) {
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

	_, err := newRESTClient()
	if err == nil {
		t.Fatal("expected newRESTClient to fail with no auth token available")
	}
	if !errors.Is(err, ErrClientInit) {
		t.Fatalf("error %v does not match ErrClientInit", err)
	}
}

func TestCheckoutPRAppliesDeadline(t *testing.T) {
	origExec := execContext
	t.Cleanup(func() { execContext = origExec })
	origRepo := currentRepo
	t.Cleanup(func() { currentRepo = origRepo })

	currentRepo = func() (repository.Repository, error) {
		return repository.Repository{Owner: "octocat", Name: "hello"}, nil
	}

	var deadline time.Time
	var hasDeadline bool
	var gotArgs []string
	execContext = func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
		deadline, hasDeadline = ctx.Deadline()
		gotArgs = args
		return bytes.Buffer{}, bytes.Buffer{}, nil
	}

	if err := CheckoutPR("octocat", "hello", 7); err != nil {
		t.Fatalf("CheckoutPR returned error: %v", err)
	}
	if !hasDeadline {
		t.Fatal("expected CheckoutPR to pass a context with a deadline")
	}
	if d := time.Until(deadline); d < SubprocessTimeout-2*time.Second || d > SubprocessTimeout+time.Second {
		t.Fatalf("deadline in %v, want ~%v", d, SubprocessTimeout)
	}
	if want := []string{"pr", "checkout", "7", "--repo", "octocat/hello"}; !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}

func TestOpenPRInBrowserAppliesDeadline(t *testing.T) {
	orig := execContext
	t.Cleanup(func() { execContext = orig })

	var deadline time.Time
	var hasDeadline bool
	var gotArgs []string
	execContext = func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
		deadline, hasDeadline = ctx.Deadline()
		gotArgs = args
		return bytes.Buffer{}, bytes.Buffer{}, nil
	}

	if err := OpenPRInBrowser("octocat", "hello", 9); err != nil {
		t.Fatalf("OpenPRInBrowser returned error: %v", err)
	}
	if !hasDeadline {
		t.Fatal("expected OpenPRInBrowser to pass a context with a deadline")
	}
	if d := time.Until(deadline); d < SubprocessTimeout-2*time.Second || d > SubprocessTimeout+time.Second {
		t.Fatalf("deadline in %v, want ~%v", d, SubprocessTimeout)
	}
	if want := []string{"pr", "view", "9", "--repo", "octocat/hello", "--web"}; !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}

func TestRESTClientOptionsEnablesTUISafeLogging(t *testing.T) {
	opts := restClientOptions()
	if !opts.LogIgnoreEnv {
		t.Error("LogIgnoreEnv = false, want true (GH_DEBUG must not write to stderr)")
	}
	if _, ok := opts.Transport.(loggingTransport); !ok {
		t.Errorf("Transport is %T, want loggingTransport", opts.Transport)
	}
}

func TestCheckoutPRLogsSuccessAndFailure(t *testing.T) {
	origExec := execContext
	t.Cleanup(func() { execContext = origExec })
	origRepo := currentRepo
	t.Cleanup(func() { currentRepo = origRepo })
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	currentRepo = func() (repository.Repository, error) {
		return repository.Repository{Owner: "octocat", Name: "hello"}, nil
	}

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	execContext = func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
		return bytes.Buffer{}, bytes.Buffer{}, nil
	}
	if err := CheckoutPR("octocat", "hello", 7); err != nil {
		t.Fatalf("CheckoutPR success returned error: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "level=INFO") || !strings.Contains(out, "checked out pr") || !strings.Contains(out, "pr=7") {
		t.Fatalf("success log missing; got: %s", out)
	}

	buf.Reset()
	execContext = func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
		return bytes.Buffer{}, bytes.Buffer{}, errors.New("boom")
	}
	if err := CheckoutPR("octocat", "hello", 7); err == nil {
		t.Fatal("expected CheckoutPR to return the exec error")
	}
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "checkout failed") || !strings.Contains(out, "boom") {
		t.Fatalf("failure log missing; got: %s", out)
	}
}

func TestOpenPRInBrowserLogsSuccessAndFailure(t *testing.T) {
	origExec := execContext
	t.Cleanup(func() { execContext = origExec })
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	execContext = func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
		return bytes.Buffer{}, bytes.Buffer{}, nil
	}
	if err := OpenPRInBrowser("octocat", "hello", 9); err != nil {
		t.Fatalf("OpenPRInBrowser success returned error: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "level=INFO") || !strings.Contains(out, "opened pr in browser") || !strings.Contains(out, "pr=9") {
		t.Fatalf("success log missing; got: %s", out)
	}

	buf.Reset()
	execContext = func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
		return bytes.Buffer{}, bytes.Buffer{}, errors.New("boom")
	}
	if err := OpenPRInBrowser("octocat", "hello", 9); err == nil {
		t.Fatal("expected OpenPRInBrowser to return the exec error")
	}
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "open in browser failed") || !strings.Contains(out, "boom") {
		t.Fatalf("failure log missing; got: %s", out)
	}
}

func TestCheckoutPRRefusesNonLocalRepo(t *testing.T) {
	origExec := execContext
	t.Cleanup(func() { execContext = origExec })
	origRepo := currentRepo
	t.Cleanup(func() { currentRepo = origRepo })
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	currentRepo = func() (repository.Repository, error) {
		return repository.Repository{Owner: "someone", Name: "other"}, nil
	}
	called := false
	execContext = func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
		called = true
		return bytes.Buffer{}, bytes.Buffer{}, nil
	}

	err := CheckoutPR("octocat", "hello", 7)
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
	origExec := execContext
	t.Cleanup(func() { execContext = origExec })
	origRepo := currentRepo
	t.Cleanup(func() { currentRepo = origRepo })
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	currentRepo = func() (repository.Repository, error) {
		return repository.Repository{}, errors.New("no git remotes configured")
	}
	called := false
	execContext = func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
		called = true
		return bytes.Buffer{}, bytes.Buffer{}, nil
	}

	err := CheckoutPR("octocat", "hello", 7)
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
	origExec := execContext
	t.Cleanup(func() { execContext = origExec })
	origRepo := currentRepo
	t.Cleanup(func() { currentRepo = origRepo })

	currentRepo = func() (repository.Repository, error) {
		return repository.Repository{Owner: "OctoCat", Name: "Hello"}, nil
	}
	var gotArgs []string
	execContext = func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error) {
		gotArgs = args
		return bytes.Buffer{}, bytes.Buffer{}, nil
	}

	if err := CheckoutPR("octocat", "hello", 7); err != nil {
		t.Fatalf("CheckoutPR returned error: %v", err)
	}
	if want := []string{"pr", "checkout", "7", "--repo", "octocat/hello"}; !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}
