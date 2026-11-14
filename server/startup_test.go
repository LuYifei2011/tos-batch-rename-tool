package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStartupPreservesCorruptHistoryForInspection(t *testing.T) {
	data := t.TempDir()
	history := filepath.Join(data, "history")
	if err := os.Mkdir(history, 0700); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(history, "0123456789abcdef0123456789abcdef.json")
	write(t, record, "incomplete JSON")
	if _, err := newStore(data, nil, 4); err == nil {
		t.Fatal("corrupt history silently accepted")
	}
	if content, _ := os.ReadFile(record); string(content) != "incomplete JSON" {
		t.Fatal("corrupt history was removed or overwritten")
	}
}
