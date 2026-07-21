package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// CheckStatus is a pull request's aggregate CI/checks state, rolled up from its head
// commit's check runs and commit statuses.
type CheckStatus int

// CheckStatus values. CheckNone means no checks were reported (rendered neutral).
const (
	CheckNone    CheckStatus = iota // no checks reported
	CheckPassing                    // every check finished, none failing
	CheckFailing                    // at least one check failing
	CheckPending                    // at least one check still running, none failing
)

// prCheckRow is one PR row of `gh pr list --json number,statusCheckRollup`.
type prCheckRow struct {
	Number            int          `json:"number"`
	StatusCheckRollup []rollupItem `json:"statusCheckRollup"` // null/empty when no checks
}

// rollupItem is one entry of a PR's statusCheckRollup. gh returns a heterogeneous
// array: CheckRun entries (GitHub Actions and other checks apps) carry
// Status+Conclusion; StatusContext entries (legacy commit statuses) carry State.
// __typename discriminates the two.
type rollupItem struct {
	Typename   string `json:"__typename"` // "CheckRun" | "StatusContext"
	Status     string `json:"status"`     // CheckRun: QUEUED/IN_PROGRESS/COMPLETED/...
	Conclusion string `json:"conclusion"` // CheckRun: SUCCESS/FAILURE/NEUTRAL/SKIPPED/...
	State      string `json:"state"`      // StatusContext: SUCCESS/PENDING/FAILURE/ERROR/EXPECTED
}

// checksFetchLimit caps how many PRs `gh pr list` reports statuses for. It sits
// comfortably above GitHub's default open-PR page size, so every PR shown in the
// list has a status row; extra rows are harmlessly ignored on join by number.
const checksFetchLimit = 100

// RepoPRChecks returns each open PR's aggregate CheckStatus, keyed by PR number, for
// owner/name. It shells out once (the same exec seam as the PR actions) and rolls up
// each PR's checks with bucketRollup. It is not cached (statuses are volatile). PRs
// absent from the result — or with no checks — are simply not in the map; the TUI
// renders a missing key as CheckNone (the map's zero value).
func (c *Client) RepoPRChecks(ctx context.Context, owner, name string) (map[int]CheckStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, c.subprocessTimeout)
	defer cancel()
	repo := fmt.Sprintf("%s/%s", owner, name)
	stdout, stderr, err := c.exec(ctx,
		"pr", "list", "--repo", repo, "--state", "open",
		"--json", "number,statusCheckRollup", "--limit", strconv.Itoa(checksFetchLimit))
	if err != nil {
		// Fold gh's stderr into the error BEFORE logging, so the Warn line (visible at
		// the default level) carries the actual failure cause, not a bare "exit status 1".
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			err = fmt.Errorf("%w: %s", err, detail)
		}
		slog.Warn("fetch pr checks failed", "repo", repo, "err", err)
		return nil, err
	}
	var rows []prCheckRow
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		slog.Warn("parse pr checks failed", "repo", repo, "err", err)
		return nil, fmt.Errorf("parse gh pr list checks: %w", err)
	}
	out := make(map[int]CheckStatus, len(rows))
	for _, r := range rows {
		out[r.Number] = bucketRollup(r.StatusCheckRollup)
	}
	slog.Debug("fetched pr checks", "repo", repo, "prs", len(out))
	return out, nil
}

// bucketRollup collapses a PR's statusCheckRollup into a single CheckStatus. Priority
// is failing > pending > passing: any failing check makes the PR failing; else any
// still-running check makes it pending; else (all finished, none failing — success,
// neutral, or skipped) it is passing. An empty rollup is CheckNone (no checks ran).
func bucketRollup(items []rollupItem) CheckStatus {
	if len(items) == 0 {
		return CheckNone
	}
	pending := false
	for _, it := range items {
		switch {
		case it.isFailing():
			return CheckFailing // failing dominates
		case it.isPending():
			pending = true
		}
	}
	if pending {
		return CheckPending
	}
	return CheckPassing
}

// isFailing reports a terminal, unsuccessful result.
func (it rollupItem) isFailing() bool {
	if it.Typename == "StatusContext" {
		return it.State == "FAILURE" || it.State == "ERROR"
	}
	// CheckRun (and any check-run-shaped item).
	if it.Status != "COMPLETED" {
		return false // not finished → handled by isPending, not failing
	}
	switch it.Conclusion {
	case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE", "STALE":
		return true
	default: // SUCCESS, NEUTRAL, SKIPPED, "" → not failing
		return false
	}
}

// isPending reports a check with no terminal result yet.
func (it rollupItem) isPending() bool {
	if it.Typename == "StatusContext" {
		return it.State == "PENDING" || it.State == "EXPECTED"
	}
	return it.Status != "COMPLETED" // CheckRun: QUEUED/IN_PROGRESS/WAITING/...
}
