package screen

import (
	"errors"
	"testing"
)

func TestFetchErrMsgTargetView(t *testing.T) {
	msg := FetchErrMsg{View: ViewPR, Err: errors.New("boom")}
	if msg.TargetView() != ViewPR {
		t.Fatalf("TargetView() = %d, want %d", msg.TargetView(), ViewPR)
	}
}
