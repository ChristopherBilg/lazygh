package tui

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ChristopherBilg/lazygh/internal/config"
	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
	"github.com/ChristopherBilg/lazygh/internal/tui/repolist"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
)

// TestMain silences the router's slog output by default so test runs stay
// clean; individual tests that assert on log output install their own logger.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

// newTestModel builds a router with a real github client. Router tests drive
// navigation with synthetic messages and never execute a fetch, so the client
// never touches the network.
func newTestModel() Model {
	return NewModel(ghClient.NewClient(ghClient.ClientConfig{}))
}

func TestRepoSelectedSwitchesToPRView(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	updated, cmd := m.Update(repolist.RepoSelectedMsg{Owner: "octocat", Name: "hello"})
	rm := updated.(Model)
	if rm.active != viewPR {
		t.Fatal("expected active view to be viewPR after RepoSelectedMsg")
	}
	if rm.perRepo[viewPR] == nil {
		t.Fatal("expected the PR screen to be built after RepoSelectedMsg")
	}
	if cmd == nil {
		t.Fatal("expected an init command (PR fetch) after selection")
	}
}

func TestEscReturnsToRepoList(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	updated, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	afterSelect := updated.(Model)
	if afterSelect.active != viewPR {
		t.Fatal("precondition: should be in PR view")
	}
	back, _ := afterSelect.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if back.(Model).active != viewRepoList {
		t.Fatal("expected esc to return to repo list")
	}
}

func TestErrMsgSetsError(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	updated, _ := m.Update(screen.ErrMsg{Err: errors.New("boom")})
	if err := updated.(Model).err; err == nil || err.Error() != "boom" {
		t.Fatalf("expected err \"boom\", got %v", err)
	}
}

func TestWindowSizeStored(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 123, Height: 45})
	rm := updated.(Model)
	if rm.width != 123 || rm.height != 45 {
		t.Fatalf("dimensions = %dx%d, want 123x45", rm.width, rm.height)
	}
}

func TestQuitOnQ(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command on 'q'")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected command to be tea.Quit")
	}
}

func TestForwardRoutesNonGlobalKeyToRepoList(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	// A non-global key (not q/ctrl+c/esc/backspace) must be forwarded to the
	// active screen via forward(); the router itself must not change views.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if updated.(Model).active != viewRepoList {
		t.Fatal("expected to stay on repo list after a non-global key")
	}
}

func TestForwardRoutesToActivePRScreen(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	selected, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	// A non-global key while in PR view is forwarded to the PR screen; the
	// router stays in PR view.
	updated, _ := selected.(Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated.(Model).active != viewPR {
		t.Fatal("expected to stay in PR view after forwarding a non-global key")
	}
}

func TestViewShowsErrorOverlay(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	errored, _ := m.Update(screen.ErrMsg{Err: errors.New("boom")})
	if view := errored.(Model).View(); !strings.Contains(view, "Error: boom") {
		t.Fatalf("expected error overlay, got %q", view)
	}
}

func TestViewDelegatesToRepoList(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	// No error, repo list active: View delegates to the repo-list screen,
	// which shows its loading text initially.
	if view := m.View(); !strings.Contains(view, "Fetching your repositories") {
		t.Fatalf("expected repo-list view, got %q", view)
	}
}

func TestViewDelegatesToPRScreen(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	selected, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "hello"})
	// PR view active, no error: View delegates to the PR screen (loading text).
	if view := selected.(Model).View(); !strings.Contains(view, "Fetching PRs for hello") {
		t.Fatalf("expected PR screen view, got %q", view)
	}
}

func TestResizeAfterSelectionReachesPRScreen(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	selected, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	resized, _ := selected.(Model).Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	rm := resized.(Model)
	if rm.width != 120 || rm.height != 50 {
		t.Fatalf("dimensions = %dx%d, want 120x50", rm.width, rm.height)
	}
	if rm.perRepo[viewPR] == nil {
		t.Fatal("expected the PR screen to remain built after resize")
	}
}

func TestNumberKeysSwitchViewsAfterSelection(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	sel, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	cur := sel.(Model)

	tests := []struct {
		key  rune
		want view
	}{
		{'2', viewIssues},
		{'3', viewActions},
		{'1', viewPR},
	}
	for _, tt := range tests {
		updated, _ := cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tt.key}})
		cur = updated.(Model)
		if cur.active != tt.want {
			t.Fatalf("after key %q, active = %d, want %d", tt.key, cur.active, tt.want)
		}
	}
}

func TestNumberKeysIgnoredOnRepoList(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if updated.(Model).active != viewRepoList {
		t.Fatal("expected number keys to be ignored on the repo list")
	}
}

func TestSwitchingViewsDoesNotReinit(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	sel, cmd := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	if cmd == nil {
		t.Fatal("expected an Init command when the per-repo screens are built")
	}
	// Switching to an already-built view must not re-init it (that is what
	// preserves its selection/scroll state).
	_, cmd = sel.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if cmd != nil {
		t.Fatal("expected no Init command when switching between held views")
	}
}

func TestAllPerRepoScreensBuiltOnSelection(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	sel, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	rm := sel.(Model)
	for _, v := range perRepoViews {
		if rm.perRepo[v] == nil {
			t.Fatalf("expected per-repo screen for view %d to be built", v)
		}
	}
}

// recorderScreen is a test double that records the messages it receives, used
// to prove the router delivers addressed results to a specific (non-active) view.
type recorderScreen struct{ got []tea.Msg }

func (r *recorderScreen) Init() tea.Cmd { return nil }
func (r *recorderScreen) Update(msg tea.Msg) (screen.Model, tea.Cmd) {
	r.got = append(r.got, msg)
	return r, nil
}
func (r *recorderScreen) View() string { return "" }

func TestAddressedMsgRoutedToOriginatingView(t *testing.T) {
	t.Parallel()
	prRec := &recorderScreen{}
	issuesRec := &recorderScreen{}
	m := Model{
		active:  viewIssues, // a DIFFERENT view is active
		perRepo: map[view]screen.Model{viewPR: prRec, viewIssues: issuesRec},
	}
	m.Update(screen.FetchErrMsg{View: screen.ViewPR, Err: errors.New("boom")})
	if len(prRec.got) != 1 {
		t.Fatalf("PR view received %d messages, want 1 (addressed delivery failed)", len(prRec.got))
	}
	if fe, ok := prRec.got[0].(screen.FetchErrMsg); !ok || fe.Err == nil || fe.Err.Error() != "boom" {
		t.Fatalf("PR view got %#v, want the FetchErrMsg with err \"boom\"", prRec.got[0])
	}
	if len(issuesRec.got) != 0 {
		t.Fatalf("Issues view (active) received %d messages, want 0", len(issuesRec.got))
	}
}

func TestErrMsgLogsError(t *testing.T) {
	// No t.Parallel: slog.SetDefault mutates the process-wide default logger.
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	m := newTestModel()
	m.Update(screen.ErrMsg{Err: errors.New("boom")})
	if out := buf.String(); !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "fatal view error") || !strings.Contains(out, "boom") {
		t.Fatalf("expected fatal error log, got: %s", out)
	}
}

func TestFetchErrMsgLogsWarning(t *testing.T) {
	// No t.Parallel: slog.SetDefault mutates the process-wide default logger.
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	m := Model{
		active:  viewIssues,
		perRepo: map[view]screen.Model{viewPR: &recorderScreen{}, viewIssues: &recorderScreen{}},
	}
	m.Update(screen.FetchErrMsg{View: screen.ViewPR, Err: errors.New("nope")})
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "view fetch error") || !strings.Contains(out, "view=1") || !strings.Contains(out, "nope") {
		t.Fatalf("expected fetch error warning log, got: %s", out)
	}
}

func TestCtrlCAlwaysQuitsEvenWhenQuitRemapped(t *testing.T) {
	// No t.Parallel: keys.Configure mutates the process-wide key map.
	t.Cleanup(func() { keys.Configure(config.Default().Keys) })
	kc := config.Default().Keys
	kc.Quit = []string{"x"} // move quit away from q
	keys.Configure(kc)

	m := newTestModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c must always quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected ctrl+c to produce tea.Quit")
	}

	_, cmd2 := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd2 == nil {
		t.Fatal("remapped quit key 'x' should quit")
	}
	if _, ok := cmd2().(tea.QuitMsg); !ok {
		t.Fatal("expected remapped quit key to produce tea.Quit")
	}

	// The old default quit key is now inert (forwarded, not a quit).
	_, cmdOld := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmdOld != nil {
		if _, ok := cmdOld().(tea.QuitMsg); ok {
			t.Fatal("'q' should no longer quit after quit is remapped to 'x'")
		}
	}
}

// capturingScreen is a screen.Model test double whose CapturingInput value is
// controllable, used to prove the router suppresses global keys while a screen is
// capturing text input.
type capturingScreen struct {
	capturing bool
	got       []string
}

func (c *capturingScreen) Init() tea.Cmd { return nil }
func (c *capturingScreen) Update(msg tea.Msg) (screen.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		c.got = append(c.got, k.String())
	}
	return c, nil
}
func (c *capturingScreen) View() string         { return "" }
func (c *capturingScreen) CapturingInput() bool { return c.capturing }

func TestCapturingScreenSuppressesGlobalKeys(t *testing.T) {
	t.Parallel()
	cs := &capturingScreen{capturing: true}
	m := Model{active: viewPR, perRepo: map[view]screen.Model{viewPR: cs}}

	// "1" would normally switch views; while capturing it must be forwarded.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if updated.(Model).active != viewPR {
		t.Fatal("global nav must be suppressed while capturing")
	}
	// "q" would normally quit; while capturing it must not.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("'q' must not quit while capturing input")
		}
	}
	// esc would normally go back; while capturing it must not.
	back, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if back.(Model).active != viewPR {
		t.Fatal("esc must not navigate back while capturing input")
	}
	if len(cs.got) != 3 {
		t.Fatalf("screen received %d keys, want 3 (all forwarded)", len(cs.got))
	}
}

func TestCtrlCQuitsEvenWhileCapturing(t *testing.T) {
	t.Parallel()
	cs := &capturingScreen{capturing: true}
	m := Model{active: viewPR, perRepo: map[view]screen.Model{viewPR: cs}}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c must quit even while capturing")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected ctrl+c to produce tea.Quit while capturing")
	}
}

func TestGlobalKeysWorkWhenNotCapturing(t *testing.T) {
	t.Parallel()
	cs := &capturingScreen{capturing: false}
	m := Model{active: viewPR, perRepo: map[view]screen.Model{viewPR: cs, viewIssues: &recorderScreen{}}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if updated.(Model).active != viewIssues {
		t.Fatal("expected '2' to switch to Issues when not capturing")
	}
}
