package outputstore

import (
	"sync"
	"time"
)

// OutputLine represents a single captured line of process output.
type OutputLine struct {
	Timestamp time.Time `json:"timestamp"`
	Content   string    `json:"content"`
	IsStdErr  bool      `json:"isStdErr"`
}

// OutputStore is a thread-safe, append-only log of process output lines.
type OutputStore struct {
	lines []OutputLine
	mu    sync.RWMutex
}

// New creates a new empty OutputStore.
func New() *OutputStore {
	return &OutputStore{
		lines: make([]OutputLine, 0),
	}
}

// AddLine appends a new output line with the current timestamp.
func (s *OutputStore) AddLine(content string, isStdErr bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, OutputLine{
		Timestamp: time.Now(),
		Content:   content,
		IsStdErr:  isStdErr,
	})
}

// GetLinesBetween returns all lines with timestamps in [start, end].
func (s *OutputStore) GetLinesBetween(start, end time.Time) []OutputLine {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []OutputLine
	for _, line := range s.lines {
		if (line.Timestamp.Equal(start) || line.Timestamp.After(start)) &&
			(line.Timestamp.Equal(end) || line.Timestamp.Before(end)) {
			result = append(result, line)
		}
	}
	return result
}

// GetLatestLines returns the most recent n lines. If n exceeds the total
// number of stored lines, all lines are returned.
func (s *OutputStore) GetLatestLines(n int) []OutputLine {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.lines)
	if n <= 0 {
		return nil
	}
	if n > total {
		n = total
	}
	result := make([]OutputLine, n)
	copy(result, s.lines[total-n:])
	return result
}

// Len returns the total number of stored lines.
func (s *OutputStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.lines)
}
