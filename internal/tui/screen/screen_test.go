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

func TestGenStampGeneration(t *testing.T) {
	if got := (GenStamp{Gen: 5}).Generation(); got != 5 {
		t.Fatalf("Generation() = %d, want 5", got)
	}
}

func TestFetchErrMsgIsGenerational(t *testing.T) {
	var msg any = FetchErrMsg{GenStamp: GenStamp{Gen: 3}, View: ViewPR, Err: errors.New("boom")}
	g, ok := msg.(Generational)
	if !ok {
		t.Fatal("FetchErrMsg must implement Generational")
	}
	if got := g.Generation(); got != 3 {
		t.Fatalf("Generation() = %d, want 3", got)
	}
}

func TestFetchErrMsgZeroGeneration(t *testing.T) {
	if got := (FetchErrMsg{View: ViewPR}).Generation(); got != 0 {
		t.Fatalf("zero-value FetchErrMsg Generation() = %d, want 0", got)
	}
}
