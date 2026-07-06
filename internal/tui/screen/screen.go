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

// ErrMsg reports a fatal error up to the router, which renders a global error
// overlay until the user quits.
type ErrMsg struct{ Err error }
