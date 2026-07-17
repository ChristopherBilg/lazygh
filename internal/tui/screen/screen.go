// Package screen defines the contract every routable sub-model implements,
// plus the cross-cutting messages the router handles centrally.
package screen

import tea "github.com/charmbracelet/bubbletea"

// Model is a self-contained, Elm-style sub-model hosted by the router. Only the
// router implements bubbletea's tea.Model; child screens implement this
// interface instead, so the router can forward messages generically without
// type assertions.
type Model interface {
	Init() tea.Cmd
	Update(tea.Msg) (Model, tea.Cmd)
	View() string
}

// ViewID identifies a top-level screen. It lives here (rather than in the
// router) so sub-models can address their async results to a specific view
// without importing the router package.
type ViewID int

// View identifiers for the router's top-level screens, in tab order.
const (
	ViewRepoList ViewID = iota
	ViewPR
	ViewIssues
	ViewActions
)

// Addressed is implemented by async result messages that must be delivered to
// the view that originated them, even if that view is not currently active. The
// router routes any Addressed message to TargetView() instead of the active
// screen, so switching tabs mid-fetch never strands a result.
type Addressed interface {
	TargetView() ViewID
}

// InputCapturer is implemented by screens that can enter a mode where they must
// receive every key (e.g. a text filter). While CapturingInput returns true, the
// router suppresses its global key handling and forwards keys straight to the
// active screen, so keystrokes like "1", "q", or "esc" are typed into the field
// instead of switching views, quitting, or going back. Screens that never capture
// input simply do not implement it.
type InputCapturer interface {
	CapturingInput() bool
}

// ErrMsg reports an unrecoverable error up to the router, which renders a global
// error overlay until the user quits. Use it only for conditions the user
// cannot recover from in-session (e.g. the GitHub client cannot be built / no
// auth token). Recoverable, per-request failures use FetchErrMsg instead.
type ErrMsg struct{ Err error }

// FetchErrMsg reports a recoverable fetch failure for a specific view. Unlike
// ErrMsg it is non-fatal: the addressed view renders it inline, keeps any cached
// content, and lets the user retry. It implements Addressed; TargetView reports
// its View field.
type FetchErrMsg struct {
	View ViewID
	Err  error
}

// TargetView reports the view this error belongs to, so the router delivers it
// to the originating screen even when another view is active.
func (m FetchErrMsg) TargetView() ViewID { return m.View }

var _ Addressed = FetchErrMsg{}
