package github

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// entry is a single cached value plus the time it was stored. storedAt is
// recorded for a future TTL (see issue #4); nothing reads it yet.
type entry struct {
	value    any
	storedAt time.Time
}

// Cache is a thread-safe, in-memory key→value store for GitHub API responses.
// The zero value is not usable; call NewCache. Each key is namespaced per
// resource type, so every key maps to a single value type and getOrLoad's type
// assertion is safe.
//
// Invalidation policy: entries live for the process lifetime and never expire on
// their own. A forced load (getOrLoad's force parameter, reached through the
// TUI's manual "r" refresh) is the only way to replace an entry. There is no TTL
// yet: storedAt is recorded for a future TTL but is currently unread.
type Cache struct {
	mu      sync.Mutex
	entries map[string]entry
}

// NewCache returns an empty, ready-to-use cache.
func NewCache() *Cache {
	return &Cache{entries: make(map[string]entry)}
}

// get returns the cached value for key, or (nil, false) on a miss.
func (c *Cache) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	return e.value, true
}

// set stores value under key, stamped with the current time.
func (c *Cache) set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = entry{value: value, storedAt: time.Now()}
}

// getOrLoad returns the cached value for key. On a miss (or when force is true)
// it invokes load, stores the result, and returns it. The cache lock is never
// held across load, so a slow loader does not block other keys; two concurrent
// misses of the same key may both load (acceptable — see the design doc).
func getOrLoad[T any](c *Cache, key string, force bool, load func() (T, error)) (T, error) {
	if !force {
		if v, ok := c.get(key); ok {
			slog.Debug("cache hit", "key", key)
			return v.(T), nil
		}
	}
	slog.Debug("cache load", "key", key, "forced", force)
	v, err := load()
	if err != nil {
		var zero T
		return zero, err
	}
	c.set(key, v)
	return v, nil
}

// Repositories returns the authenticated user's repositories, served from the
// cache when present. force bypasses the cache and refreshes the stored entry.
func (c *Client) Repositories(ctx context.Context, force bool) ([]Repository, error) {
	return getOrLoad(c.cache, "repos", force, func() ([]Repository, error) {
		return c.FetchUserRepositories(ctx)
	})
}

// RepoPRs returns the given repository's pull requests, served from the cache
// when present. force bypasses the cache and refreshes the stored entry.
func (c *Client) RepoPRs(ctx context.Context, owner, name string, force bool) (RepoContext, error) {
	key := "prs:" + owner + "/" + name
	return getOrLoad(c.cache, key, force, func() (RepoContext, error) {
		return c.FetchRepoPRs(ctx, owner, name)
	})
}

// PRComments returns the given PR's conversation comments, served from the cache
// when present. force bypasses the cache and refreshes the stored entry.
func (c *Client) PRComments(ctx context.Context, owner, name string, prNumber int, force bool) ([]PRComment, error) {
	key := fmt.Sprintf("pr-comments:%s/%s#%d", owner, name, prNumber)
	return getOrLoad(c.cache, key, force, func() ([]PRComment, error) {
		return c.FetchPRComments(ctx, owner, name, prNumber)
	})
}

// PRDiff returns the given PR's unified diff, served from the cache when present.
// force bypasses the cache and refreshes the stored entry.
func (c *Client) PRDiff(ctx context.Context, owner, name string, prNumber int, force bool) (string, error) {
	key := fmt.Sprintf("pr-diff:%s/%s#%d", owner, name, prNumber)
	return getOrLoad(c.cache, key, force, func() (string, error) {
		return c.FetchPRDiff(ctx, owner, name, prNumber)
	})
}
