package outputstore

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAddAndGetLinesBetween(t *testing.T) {
	store := New()
	start := time.Now()

	store.AddLine("stdout line 1", false)
	store.AddLine("stderr line", true)
	store.AddLine("stdout line 2", false)

	end := time.Now()

	lines := store.GetLinesBetween(start, end)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	if lines[0].Content != "stdout line 1" || lines[0].IsStdErr {
		t.Error("first line mismatch")
	}
	if lines[1].Content != "stderr line" || !lines[1].IsStdErr {
		t.Error("second line mismatch")
	}
	if lines[2].Content != "stdout line 2" || lines[2].IsStdErr {
		t.Error("third line mismatch")
	}
}

func TestGetLinesBetweenFiltersCorrectly(t *testing.T) {
	store := New()

	store.AddLine("before", false)
	time.Sleep(10 * time.Millisecond)

	start := time.Now()
	time.Sleep(10 * time.Millisecond)
	store.AddLine("during", false)
	time.Sleep(10 * time.Millisecond)
	end := time.Now()

	time.Sleep(10 * time.Millisecond)
	store.AddLine("after", false)

	lines := store.GetLinesBetween(start, end)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0].Content != "during" {
		t.Errorf("expected 'during', got %q", lines[0].Content)
	}
}

func TestGetLatestLines(t *testing.T) {
	store := New()
	store.AddLine("line 1", false)
	store.AddLine("line 2", false)
	store.AddLine("line 3", false)
	store.AddLine("line 4", false)
	store.AddLine("line 5", false)

	lines := store.GetLatestLines(3)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0].Content != "line 3" {
		t.Errorf("expected 'line 3', got %q", lines[0].Content)
	}
	if lines[2].Content != "line 5" {
		t.Errorf("expected 'line 5', got %q", lines[2].Content)
	}
}

func TestGetLatestLinesMoreThanAvailable(t *testing.T) {
	store := New()
	store.AddLine("only line", false)

	lines := store.GetLatestLines(100)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
}

func TestGetLatestLinesZero(t *testing.T) {
	store := New()
	store.AddLine("something", false)

	lines := store.GetLatestLines(0)
	if lines != nil {
		t.Fatalf("expected nil, got %v", lines)
	}
}

func TestJSONSerialization(t *testing.T) {
	store := New()
	store.AddLine("hello", false)
	store.AddLine("world", true)

	lines := store.GetLatestLines(2)
	data, err := json.Marshal(lines)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}

	var decoded []OutputLine
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	if len(decoded) != 2 {
		t.Fatalf("expected 2 decoded lines, got %d", len(decoded))
	}
	if decoded[0].Content != "hello" || decoded[0].IsStdErr {
		t.Error("first decoded line mismatch")
	}
	if decoded[1].Content != "world" || !decoded[1].IsStdErr {
		t.Error("second decoded line mismatch")
	}
}

func TestLen(t *testing.T) {
	store := New()
	if store.Len() != 0 {
		t.Fatal("expected 0 length for new store")
	}
	store.AddLine("a", false)
	store.AddLine("b", true)
	if store.Len() != 2 {
		t.Fatalf("expected 2, got %d", store.Len())
	}
}
