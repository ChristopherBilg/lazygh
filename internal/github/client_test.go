package github

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"
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

	if err := CheckoutPR(7); err != nil {
		t.Fatalf("CheckoutPR returned error: %v", err)
	}
	if !hasDeadline {
		t.Fatal("expected CheckoutPR to pass a context with a deadline")
	}
	if d := time.Until(deadline); d < SubprocessTimeout-2*time.Second || d > SubprocessTimeout+time.Second {
		t.Fatalf("deadline in %v, want ~%v", d, SubprocessTimeout)
	}
	if want := []string{"pr", "checkout", "7"}; !slices.Equal(gotArgs, want) {
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

	if err := OpenPRInBrowser(9); err != nil {
		t.Fatalf("OpenPRInBrowser returned error: %v", err)
	}
	if !hasDeadline {
		t.Fatal("expected OpenPRInBrowser to pass a context with a deadline")
	}
	if d := time.Until(deadline); d < SubprocessTimeout-2*time.Second || d > SubprocessTimeout+time.Second {
		t.Fatalf("deadline in %v, want ~%v", d, SubprocessTimeout)
	}
	if want := []string{"pr", "view", "9", "--web"}; !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}
