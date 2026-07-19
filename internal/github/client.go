// Package github wraps the GitHub REST API and `gh` subprocess calls lazygh
// needs (repository listing, pull requests, PR comments, PR diffs, checkout, open-in-browser), with a
// small in-memory response cache. All outbound calls are bounded by timeouts.
package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
)

// ClientConfig holds the tunables for a Client. Zero/negative fields fall back
// to built-in defaults in NewClient.
type ClientConfig struct {
	RESTTimeout       time.Duration
	SubprocessTimeout time.Duration
	RepoPageSize      int
}

// Client performs GitHub REST and `gh` subprocess calls, backed by an in-memory
// response cache. Construct it with NewClient; the zero value is not usable.
type Client struct {
	restTimeout       time.Duration
	subprocessTimeout time.Duration
	pageSize          int
	cache             *Cache
	// Seams, defaulted to the real implementations; tests override them.
	exec        func(ctx context.Context, args ...string) (stdout, stderr bytes.Buffer, err error)
	currentRepo func() (repository.Repository, error)
	fetchLogin  func(ctx context.Context) (string, error)
}

// Built-in defaults, applied by NewClient for any non-positive ClientConfig field.
const (
	defaultRESTTimeout       = 10 * time.Second
	defaultSubprocessTimeout = 30 * time.Second
	defaultRepoPageSize      = 50
)

// NewClient returns a ready Client, applying defaults for non-positive fields.
func NewClient(cfg ClientConfig) *Client {
	c := &Client{
		restTimeout:       cfg.RESTTimeout,
		subprocessTimeout: cfg.SubprocessTimeout,
		pageSize:          cfg.RepoPageSize,
		cache:             NewCache(),
		exec:              gh.ExecContext,
		currentRepo:       repository.Current,
	}
	if c.restTimeout <= 0 {
		c.restTimeout = defaultRESTTimeout
	}
	if c.subprocessTimeout <= 0 {
		c.subprocessTimeout = defaultSubprocessTimeout
	}
	if c.pageSize <= 0 {
		c.pageSize = defaultRepoPageSize
	}
	c.fetchLogin = c.fetchLoginREST
	return c
}

// ErrClientInit wraps a failure to construct the REST client (e.g. no auth
// token resolvable). It is unrecoverable within the session, so the TUI treats
// it as fatal, unlike ordinary request errors.
var ErrClientInit = errors.New("github client init failed")

// ErrNotLocalRepo indicates the selected repository is not the one lazygh's
// working directory is a clone of. CheckoutPR returns it instead of running
// `gh pr checkout`, because checkout fetches the PR branch into the current git
// tree and checking out another repository's PR there is not meaningful.
var ErrNotLocalRepo = errors.New("selected repository is not lazygh's local repository")

// Repository is a GitHub repository as returned by the REST API's repo endpoints.
type Repository struct {
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
}

// User is the subset of a GitHub user object lazygh needs: the login handle.
type User struct {
	Login string `json:"login"`
}

// PullRequest is a single pull request as returned by the REST API.
type PullRequest struct {
	Title              string `json:"title"`
	Number             int    `json:"number"`
	State              string `json:"state"`
	Body               string `json:"body"`
	User               User   `json:"user"`                // author
	RequestedReviewers []User `json:"requested_reviewers"` // pending review requests
}

// PRComment is a single conversation comment on a pull request, as returned by
// the issues/{n}/comments REST endpoint (a PR's conversation comments are issue
// comments in GitHub's API).
type PRComment struct {
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// RepoContext pairs a repository's owner/name with its fetched pull requests.
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
func (c *Client) restClientOptions() api.ClientOptions {
	return api.ClientOptions{
		Timeout: c.restTimeout,
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
func (c *Client) newRESTClient() (*api.RESTClient, error) {
	client, err := api.NewRESTClient(c.restClientOptions())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrClientInit, err)
	}
	return client, nil
}

// reposEndpoint builds the repo-picker REST path; sort=pushed surfaces the most
// recently active repos first.
func reposEndpoint(pageSize int) string {
	return fmt.Sprintf("user/repos?sort=pushed&per_page=%d", pageSize)
}

// prCommentsEndpoint builds the REST path for a PR's conversation comments. PR
// conversation comments are issue comments, so this targets the issues endpoint.
// per_page=100 fetches a single large page; full pagination is future work.
func prCommentsEndpoint(owner, name string, prNumber int) string {
	return fmt.Sprintf("repos/%s/%s/issues/%d/comments?per_page=100", owner, name, prNumber)
}

// FetchUserRepositories gets the most recently pushed-to repositories for the
// authenticated user. per_page defaults to 50 and is overridable via ClientConfig.
func (c *Client) FetchUserRepositories(ctx context.Context) ([]Repository, error) {
	client, err := c.newRESTClient()
	if err != nil {
		return nil, err
	}
	var repos []Repository
	if err := client.DoWithContext(ctx, http.MethodGet, reposEndpoint(c.pageSize), nil, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// FetchRepoPRs fetches the pull requests for the given owner/name.
func (c *Client) FetchRepoPRs(ctx context.Context, owner, name string) (RepoContext, error) {
	client, err := c.newRESTClient()
	if err != nil {
		return RepoContext{}, err
	}
	endpoint := fmt.Sprintf("repos/%s/%s/pulls", owner, name)
	var prs []PullRequest
	if err := client.DoWithContext(ctx, http.MethodGet, endpoint, nil, &prs); err != nil {
		return RepoContext{}, err
	}
	return RepoContext{Owner: owner, Name: name, PRs: prs}, nil
}

// FetchPRComments fetches the conversation comments for the given PR of
// owner/name, in the API's chronological (reading) order.
func (c *Client) FetchPRComments(ctx context.Context, owner, name string, prNumber int) ([]PRComment, error) {
	client, err := c.newRESTClient()
	if err != nil {
		return nil, err
	}
	var comments []PRComment
	if err := client.DoWithContext(ctx, http.MethodGet, prCommentsEndpoint(owner, name, prNumber), nil, &comments); err != nil {
		return nil, err
	}
	return comments, nil
}

// fetchLoginREST resolves the authenticated user's login via REST GET user.
func (c *Client) fetchLoginREST(ctx context.Context) (string, error) {
	client, err := c.newRESTClient()
	if err != nil {
		return "", err
	}
	var u User
	if err := client.DoWithContext(ctx, http.MethodGet, "user", nil, &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

// CurrentUser returns the authenticated user's login, memoized for the process
// lifetime (the login does not change within a session). getOrLoad caches only
// on success, so a failed lookup is retried on the next call.
func (c *Client) CurrentUser(ctx context.Context) (string, error) {
	return getOrLoad(c.cache, "user", false, func() (string, error) {
		return c.fetchLogin(ctx)
	})
}

// CheckoutPR checks out the given PR of owner/name into the current git working
// directory. `gh pr checkout` fetches into the current tree, so it is only
// meaningful when lazygh is running inside a clone of the selected repository.
// When the working directory is not that repository (or is not a resolvable git
// repository at all), CheckoutPR returns ErrNotLocalRepo without running `gh`.
func (c *Client) CheckoutPR(ctx context.Context, owner, name string, prNumber int) error {
	repo := fmt.Sprintf("%s/%s", owner, name)

	local, err := c.currentRepo()
	if err != nil {
		slog.Warn("checkout unavailable: cannot resolve local repository", "selected", repo, "pr", prNumber, "err", err)
		return ErrNotLocalRepo
	}
	if !strings.EqualFold(local.Owner, owner) || !strings.EqualFold(local.Name, name) {
		slog.Warn("checkout unavailable: not lazygh's local repository",
			"selected", repo, "local", fmt.Sprintf("%s/%s", local.Owner, local.Name), "pr", prNumber)
		return ErrNotLocalRepo
	}

	ctx, cancel := context.WithTimeout(ctx, c.subprocessTimeout)
	defer cancel()
	_, _, err = c.exec(ctx, "pr", "checkout", strconv.Itoa(prNumber), "--repo", repo)
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
func (c *Client) OpenPRInBrowser(ctx context.Context, owner, name string, prNumber int) error {
	ctx, cancel := context.WithTimeout(ctx, c.subprocessTimeout)
	defer cancel()
	repo := fmt.Sprintf("%s/%s", owner, name)
	_, _, err := c.exec(ctx, "pr", "view", strconv.Itoa(prNumber), "--repo", repo, "--web")
	if err != nil {
		slog.Warn("open in browser failed", "repo", repo, "pr", prNumber, "err", err)
		return err
	}
	slog.Info("opened pr in browser", "repo", repo, "pr", prNumber)
	return nil
}

// FetchPRDiff fetches the unified diff for the given PR of owner/name by running
// `gh pr diff`. Passing --repo scopes gh to the selected repository (like
// OpenPRInBrowser), so it works for any selected repo, not just a local clone.
// The subprocess is not a TTY, so gh emits a plain (uncolored) unified diff, which
// is what the diff renderer highlights.
func (c *Client) FetchPRDiff(ctx context.Context, owner, name string, prNumber int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.subprocessTimeout)
	defer cancel()
	repo := fmt.Sprintf("%s/%s", owner, name)
	stdout, stderr, err := c.exec(ctx, "pr", "diff", strconv.Itoa(prNumber), "--repo", repo)
	if err != nil {
		slog.Warn("fetch pr diff failed", "repo", repo, "pr", prNumber, "err", err, "stderr", strings.TrimSpace(stderr.String()))
		return "", err
	}
	slog.Info("fetched pr diff", "repo", repo, "pr", prNumber, "bytes", stdout.Len())
	return stdout.String(), nil
}
