package github

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
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

// currentRepo resolves the GitHub repository the current working directory is a
// clone of. It is indirected so tests can substitute a stub without requiring a
// real git repository. repository.Current reads git remotes (no network) and
// honors the GH_REPO override.
var currentRepo = repository.Current

// ErrNotLocalRepo indicates the selected repository is not the one lazygh's
// working directory is a clone of. CheckoutPR returns it instead of running
// `gh pr checkout`, because checkout fetches the PR branch into the current git
// tree and checking out another repository's PR there is not meaningful.
var ErrNotLocalRepo = errors.New("selected repository is not lazygh's local repository")

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

// CheckoutPR checks out the given PR of owner/name into the current git working
// directory. `gh pr checkout` fetches into the current tree, so it is only
// meaningful when lazygh is running inside a clone of the selected repository.
// When the working directory is not that repository (or is not a resolvable git
// repository at all), CheckoutPR returns ErrNotLocalRepo without running `gh`.
func CheckoutPR(owner, name string, prNumber int) error {
	repo := fmt.Sprintf("%s/%s", owner, name)

	local, err := currentRepo()
	if err != nil {
		slog.Warn("checkout unavailable: cannot resolve local repository", "selected", repo, "pr", prNumber, "err", err)
		return ErrNotLocalRepo
	}
	if !strings.EqualFold(local.Owner, owner) || !strings.EqualFold(local.Name, name) {
		slog.Warn("checkout unavailable: not lazygh's local repository",
			"selected", repo, "local", fmt.Sprintf("%s/%s", local.Owner, local.Name), "pr", prNumber)
		return ErrNotLocalRepo
	}

	ctx, cancel := context.WithTimeout(context.Background(), SubprocessTimeout)
	defer cancel()
	_, _, err = execContext(ctx, "pr", "checkout", fmt.Sprintf("%d", prNumber), "--repo", repo)
	if err != nil {
		slog.Warn("checkout failed", "repo", repo, "pr", prNumber, "err", err)
		return err
	}
	slog.Info("checked out pr", "repo", repo, "pr", prNumber)
	return nil
}

// OpenPRInBrowser opens the given PR of owner/name in the browser. Passing
// --repo scopes `gh` to the selected repository instead of letting it resolve
// one from the current working directory, so it works for any selected repo.
func OpenPRInBrowser(owner, name string, prNumber int) error {
	ctx, cancel := context.WithTimeout(context.Background(), SubprocessTimeout)
	defer cancel()
	repo := fmt.Sprintf("%s/%s", owner, name)
	_, _, err := execContext(ctx, "pr", "view", fmt.Sprintf("%d", prNumber), "--repo", repo, "--web")
	if err != nil {
		slog.Warn("open in browser failed", "repo", repo, "pr", prNumber, "err", err)
		return err
	}
	slog.Info("opened pr in browser", "repo", repo, "pr", prNumber)
	return nil
}
