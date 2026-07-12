package github

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
)

// Timeouts bound every outbound GitHub call so a slow or unreachable endpoint
// fails within a fixed window instead of hanging indefinitely.
const (
	// RESTTimeout bounds each REST API request (repo list, PR list).
	RESTTimeout = 10 * time.Second
	// SubprocessTimeout bounds each `gh` subprocess call. Checkout runs a
	// `git fetch`, which can legitimately take longer than a REST request, so
	// it gets a more generous bound.
	SubprocessTimeout = 30 * time.Second
)

// ErrClientInit wraps a failure to construct the REST client (e.g. no auth
// token resolvable). It is unrecoverable within the session, so the TUI treats
// it as fatal, unlike ordinary request errors.
var ErrClientInit = errors.New("github client init failed")

// execContext is the subprocess runner, indirected so tests can substitute a
// stub without spawning the real `gh` binary.
var execContext = gh.ExecContext

type Repository struct {
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
}

type PullRequest struct {
	Title  string `json:"title"`
	Number int    `json:"number"`
	State  string `json:"state"`
	Body   string `json:"body"`
}

type RepoContext struct {
	Owner string
	Name  string
	PRs   []PullRequest
}

// restClientOptions returns the options every REST client is built with. Timeout
// bounds each request; Transport installs the logging round-tripper (request
// traces + rate-limit headers); LogIgnoreEnv stops go-gh from writing HTTP logs
// to stderr (which it does when GH_DEBUG is set) and corrupting the TUI. Host
// and auth token are left empty so go-gh resolves them from the gh CLI's
// configuration (optionsNeedResolution is true when Host/AuthToken are empty,
// and resolveOptions preserves the fields set here).
func restClientOptions() api.ClientOptions {
	return api.ClientOptions{
		Timeout: RESTTimeout,
		// Log every request and its rate-limit headers to the file logger.
		Transport: loggingTransport{base: http.DefaultTransport},
		// Never let GH_DEBUG make go-gh write HTTP logs to stderr and corrupt
		// the alt-screen (see go-gh pkg/api/http_client.go).
		LogIgnoreEnv: true,
	}
}

// newRESTClient builds a REST client with a bounded per-request timeout. A
// construction failure is wrapped with ErrClientInit so callers can tell an
// unrecoverable auth/config problem apart from a transient request error.
func newRESTClient() (*api.RESTClient, error) {
	client, err := api.NewRESTClient(restClientOptions())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrClientInit, err)
	}
	return client, nil
}

// FetchUserRepositories gets the 50 most recently pushed-to repositories for the authenticated user.
func FetchUserRepositories() ([]Repository, error) {
	client, err := newRESTClient()
	if err != nil {
		return nil, err
	}

	var repos []Repository
	// Sort by pushed to show the most actively developed repos first
	if err := client.Get("user/repos?sort=pushed&per_page=50", &repos); err != nil {
		return nil, err
	}

	return repos, nil
}

// FetchRepoPRs now accepts an explicit owner and name instead of guessing the local repo.
func FetchRepoPRs(owner, name string) (RepoContext, error) {
	client, err := newRESTClient()
	if err != nil {
		return RepoContext{}, err
	}

	endpoint := fmt.Sprintf("repos/%s/%s/pulls", owner, name)
	var prs []PullRequest

	if err := client.Get(endpoint, &prs); err != nil {
		return RepoContext{}, err
	}

	return RepoContext{
		Owner: owner,
		Name:  name,
		PRs:   prs,
	}, nil
}

func CheckoutPR(prNumber int) error {
	ctx, cancel := context.WithTimeout(context.Background(), SubprocessTimeout)
	defer cancel()
	_, _, err := execContext(ctx, "pr", "checkout", fmt.Sprintf("%d", prNumber))
	if err != nil {
		slog.Warn("checkout failed", "pr", prNumber, "err", err)
		return err
	}
	slog.Info("checked out pr", "pr", prNumber)
	return nil
}

func OpenPRInBrowser(prNumber int) error {
	ctx, cancel := context.WithTimeout(context.Background(), SubprocessTimeout)
	defer cancel()
	_, _, err := execContext(ctx, "pr", "view", fmt.Sprintf("%d", prNumber), "--web")
	if err != nil {
		slog.Warn("open in browser failed", "pr", prNumber, "err", err)
		return err
	}
	slog.Info("opened pr in browser", "pr", prNumber)
	return nil
}
