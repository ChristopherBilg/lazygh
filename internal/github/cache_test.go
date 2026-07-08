package github

import (
	"errors"
	"strconv"
	"sync"
	"testing"
)

func TestCacheGetMiss(t *testing.T) {
	c := NewCache()
	if _, ok := c.get("nope"); ok {
		t.Fatal("expected miss on empty cache")
	}
}

func TestGetOrLoadMissThenHit(t *testing.T) {
	c := NewCache()
	var calls int
	load := func() (int, error) {
		calls++
		return 42, nil
	}

	v, err := getOrLoad(c, "k", false, load)
	if err != nil || v != 42 {
		t.Fatalf("first call: got (%d, %v), want (42, nil)", v, err)
	}
	if calls != 1 {
		t.Fatalf("after miss: calls = %d, want 1", calls)
	}

	v, err = getOrLoad(c, "k", false, load)
	if err != nil || v != 42 {
		t.Fatalf("second call: got (%d, %v), want (42, nil)", v, err)
	}
	if calls != 1 {
		t.Fatalf("after hit: calls = %d, want 1 (loader must not run again)", calls)
	}
}

func TestGetOrLoadForceReloads(t *testing.T) {
	c := NewCache()
	var calls int
	load := func() (int, error) {
		calls++
		return calls, nil // returns 1, then 2, ...
	}

	if v, _ := getOrLoad(c, "k", false, load); v != 1 {
		t.Fatalf("first load = %d, want 1", v)
	}
	if v, _ := getOrLoad(c, "k", true, load); v != 2 {
		t.Fatalf("forced load = %d, want 2", v)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if v, _ := getOrLoad(c, "k", false, load); v != 2 {
		t.Fatalf("post-force hit = %d, want 2 (refreshed entry)", v)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (hit must not load)", calls)
	}
}

func TestGetOrLoadErrorNotCached(t *testing.T) {
	c := NewCache()
	wantErr := errors.New("boom")
	var calls int
	load := func() (int, error) {
		calls++
		return 0, wantErr
	}

	if _, err := getOrLoad(c, "k", false, load); !errors.Is(err, wantErr) {
		t.Fatalf("first call err = %v, want %v", err, wantErr)
	}
	if _, err := getOrLoad(c, "k", false, load); !errors.Is(err, wantErr) {
		t.Fatalf("second call err = %v, want %v", err, wantErr)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (error must not be cached)", calls)
	}
}

func TestGetOrLoadConcurrent(t *testing.T) {
	c := NewCache()
	load := func() (int, error) { return 7, nil }

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			// Half hammer a shared key; half use per-goroutine keys.
			key := "shared"
			if i%2 == 0 {
				key = "k" + strconv.Itoa(i)
			}
			if v, err := getOrLoad(c, key, false, load); err != nil || v != 7 {
				t.Errorf("getOrLoad(%q) = (%d, %v), want (7, nil)", key, v, err)
			}
		}()
	}
	wg.Wait()
}
