package dedup

import (
	"testing"

	"github.com/cobanov/herdr-ntfysh/internal/config"
)

func newStore(t *testing.T, window int) (*Store, *int64) {
	t.Helper()
	cfg := &config.Config{StateDir: t.TempDir(), DedupWindow: window}
	s := Open(cfg)
	var clock int64 = 1000
	s.now = func() int64 { return clock }
	return s, &clock
}

func TestDebounceSuppressesRepeat(t *testing.T) {
	s, clock := newStore(t, 10)

	if !s.ShouldNotify("p1", "done") {
		t.Fatal("first notification should be allowed")
	}
	s.Record("p1", "done")

	*clock = 1005 // within window
	if s.ShouldNotify("p1", "done") {
		t.Error("repeat within window should be suppressed")
	}

	*clock = 1011 // past window
	if !s.ShouldNotify("p1", "done") {
		t.Error("after window, notification should be allowed again")
	}
}

func TestDifferentStatusNotSuppressed(t *testing.T) {
	s, _ := newStore(t, 10)
	s.Record("p1", "working")
	if !s.ShouldNotify("p1", "done") {
		t.Error("different status should not be suppressed")
	}
}

func TestDifferentPaneNotSuppressed(t *testing.T) {
	s, _ := newStore(t, 10)
	s.Record("p1", "done")
	if !s.ShouldNotify("p2", "done") {
		t.Error("different pane should not be suppressed")
	}
}

func TestPersistenceAcrossOpens(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{StateDir: dir, DedupWindow: 10}

	s1 := Open(cfg)
	s1.now = func() int64 { return 1000 }
	s1.Record("p1", "done")

	// A fresh process/run should load the persisted state.
	s2 := Open(cfg)
	s2.now = func() int64 { return 1005 }
	if s2.ShouldNotify("p1", "done") {
		t.Error("persisted debounce state not loaded across Open calls")
	}
}

func TestDisabledWhenNoStateDir(t *testing.T) {
	s := Open(&config.Config{StateDir: "", DedupWindow: 10})
	s.Record("p1", "done")
	if !s.ShouldNotify("p1", "done") {
		t.Error("debouncing must be disabled without a state dir")
	}
}
