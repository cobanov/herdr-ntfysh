// Package dedup suppresses duplicate notifications for the same pane and
// status within a short time window. herdr can re-emit an event, and an agent
// can flap between states; without this, a single logical "done" could turn
// into a burst of pushes.
//
// State is a small JSON map persisted under HERDR_PLUGIN_STATE_DIR, the
// durable per-plugin location herdr provides. When no state dir is available
// (e.g. a standalone --test run) debouncing is disabled and every call is
// allowed.
package dedup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/cobanov/herdr-ntfysh/internal/config"
)

const stateFile = "last-status.json"

type entry struct {
	Status string `json:"status"`
	Ts     int64  `json:"ts"`
}

// Store is the persisted per-pane last-notified state.
type Store struct {
	path    string
	window  int64
	data    map[string]entry
	enabled bool
	now     func() int64
}

// Open loads existing debounce state for the current run.
func Open(cfg *config.Config) *Store {
	s := &Store{
		data:    map[string]entry{},
		window:  int64(cfg.DedupWindow),
		enabled: cfg.StateDir != "" && cfg.DedupWindow > 0,
		now:     func() int64 { return time.Now().Unix() },
	}
	if !s.enabled {
		return s
	}
	s.path = filepath.Join(cfg.StateDir, stateFile)
	if b, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(b, &s.data) // corrupt state is treated as empty
	}
	return s
}

// ShouldNotify reports whether a push for (key, status) is allowed, i.e. the
// same status was not already announced for this pane within the window.
func (s *Store) ShouldNotify(key, status string) bool {
	if !s.enabled {
		return true
	}
	if e, ok := s.data[key]; ok && e.Status == status && s.now()-e.Ts < s.window {
		return false
	}
	return true
}

// Record persists that (key, status) was just announced. It is a no-op when
// debouncing is disabled.
func (s *Store) Record(key, status string) {
	if !s.enabled {
		return
	}
	s.data[key] = entry{Status: status, Ts: s.now()}

	b, err := json.Marshal(s.data)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	// Write-then-rename so a crash never leaves a truncated state file.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}
