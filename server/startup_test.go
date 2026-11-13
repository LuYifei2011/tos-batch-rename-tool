package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareDataPreservesHistoryAndRestrictsPermissions(t *testing.T) {
	app := t.TempDir()
	data := filepath.Join(app, "data")
	if err := prepareDataDirectory(app, os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(data, "history.json"), "preserved")
	if err := os.Chmod(data, 0755); err != nil {
		t.Fatal(err)
	}
	if err := prepareDataDirectory(app, os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(data)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("data directory permissions: %v, %v", info, err)
	}
	if content, _ := os.ReadFile(filepath.Join(data, "history.json")); string(content) != "preserved" {
		t.Fatal("initialization changed history")
	}
	if err := checkTOSData(data); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareDataRejectsSymlinksAndFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "file"} {
		t.Run(kind, func(t *testing.T) {
			app := t.TempDir()
			external := t.TempDir()
			before, _ := os.Stat(external)
			data := filepath.Join(app, "data")
			if kind == "symlink" {
				if err := os.Symlink(external, data); err != nil {
					t.Fatal(err)
				}
			} else {
				write(t, data, "keep")
			}
			if err := prepareDataDirectory(app, os.Getuid(), os.Getgid()); err == nil {
				t.Fatal("unsafe data path accepted")
			}
			if err := checkTOSData(data); err == nil {
				t.Fatal("startup accepted unsafe data path")
			}
			after, _ := os.Stat(external)
			if after.Mode() != before.Mode() {
				t.Fatal("changed the external directory")
			}
		})
	}
}

func TestStartupRejectsMissingData(t *testing.T) {
	if err := checkTOSData(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing data directory accepted")
	}
}

func TestStartupRejectsUnwritableData(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires an unprivileged account")
	}
	data := t.TempDir()
	t.Cleanup(func() { os.Chmod(data, 0700) })
	if err := os.Chmod(data, 0500); err != nil {
		t.Fatal(err)
	}
	if err := checkTOSData(data); err == nil {
		t.Fatal("read-only data directory accepted")
	}
}

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
