package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/chronosarchive/chronosarchive/config"
)

const snapshotVersion = 1

// snapshot captures the full resumable state of a session to disk.
type snapshot struct {
	Version   int                      `json:"version"`
	Config    config.SessionConfig     `json:"config"`
	Turn      int                      `json:"turn"`
	StartedAt time.Time                `json:"started_at"`
	Messages  []anthropic.MessageParam `json:"messages"`
	Logs      []LogEntry               `json:"logs"`
	SavedAt   time.Time                `json:"saved_at"`
}

// snapshotPath returns <project_path>/.chronosarchive/<session-name>.json
func snapshotPath(cfg config.SessionConfig) string {
	return filepath.Join(cfg.ProjectPath, ".chronosarchive", cfg.Name+".json")
}

// saveSnapshot writes the current in-flight messages and session state to disk.
// Called after each completed turn so the session can be resumed after a restart.
func (s *Session) saveSnapshot(messages []anthropic.MessageParam) error {
	path := snapshotPath(s.Config)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	s.mu.RLock()
	snap := snapshot{
		Version:   snapshotVersion,
		Config:    s.Config,
		Turn:      s.turn,
		StartedAt: s.startedAt,
		Messages:  messages,
		Logs:      s.logs,
		SavedAt:   time.Now(),
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// loadSnapshot reads a saved snapshot for this session config.
// Returns nil (no error) when no snapshot file exists.
func loadSnapshot(cfg config.SessionConfig) (*snapshot, error) {
	data, err := os.ReadFile(snapshotPath(cfg))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	if snap.Version != snapshotVersion {
		return nil, nil // incompatible format; start fresh
	}
	return &snap, nil
}

// deleteSnapshot removes the on-disk snapshot (called on clean completion).
func (s *Session) deleteSnapshot() {
	_ = os.Remove(snapshotPath(s.Config))
}
