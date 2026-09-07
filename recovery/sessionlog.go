package recovery

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionLog tees server console output into log/<start-datetime>.txt.
// A new file is created for each server start.
type SessionLog struct {
	mu   sync.Mutex
	file *os.File
	Path string
}

// OpenSessionLog creates dir if needed and opens log/<start-datetime>.txt.
func OpenSessionLog(dir string) (*SessionLog, error) {
	if dir == "" {
		dir = "log"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	name := time.Now().Format("2006-01-02_15-04-05") + ".txt"
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open session log: %w", err)
	}
	return &SessionLog{file: f, Path: path}, nil
}

// Write implements io.Writer for log.SetOutput.
func (s *SessionLog) Write(p []byte) (int, error) {
	if s == nil || s.file == nil {
		return len(p), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file.Write(p)
}

// Close flushes and closes the session file.
func (s *SessionLog) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.file.Close()
	s.file = nil
	return err
}

// Writer returns s as an io.Writer (nil-safe).
func (s *SessionLog) Writer() io.Writer {
	if s == nil {
		return io.Discard
	}
	return s
}
