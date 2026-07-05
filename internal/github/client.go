package github

import (
	"fmt"

	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
)

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

// FetchUserRepositories gets the 50 most recently pushed-to repositories for the authenticated user.
func FetchUserRepositories() ([]Repository, error) {
	client, err := api.DefaultRESTClient()
	if err != nil {
		return nil, err
	}

	var repos []Repository
	// Sort by pushed to show the most actively developed repos first
	err = client.Get("user/repos?sort=pushed&per_page=50", &repos)
	if err != nil {
		return nil, err
	}

	return repos, nil
}

// FetchRepoPRs now accepts an explicit owner and name instead of guessing the local repo.
func FetchRepoPRs(owner, name string) (RepoContext, error) {
	client, err := api.DefaultRESTClient()
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
	_, _, err := gh.Exec("pr", "checkout", fmt.Sprintf("%d", prNumber))
	return err
}

func OpenPRInBrowser(prNumber int) error {
	_, _, err := gh.Exec("pr", "view", fmt.Sprintf("%d", prNumber), "--web")
	return err
}
