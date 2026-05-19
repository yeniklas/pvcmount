package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMountpoint_override(t *testing.T) {
	dir := t.TempDir()

	path, autoCreated, err := resolveMountpoint("my-pvc", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if autoCreated {
		t.Error("autoCreated should be false when override is given")
	}
	abs, _ := filepath.Abs(dir)
	if path != abs {
		t.Errorf("path = %q, want %q", path, abs)
	}
}

func TestResolveMountpoint_override_notExist(t *testing.T) {
	_, _, err := resolveMountpoint("my-pvc", "/nonexistent/path/xyz")
	if err == nil {
		t.Error("expected error for non-existent override path, got nil")
	}
}

func TestResolveMountpoint_override_notDir(t *testing.T) {
	f, err := os.CreateTemp("", "pvcmount-test-*")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	_, _, err = resolveMountpoint("my-pvc", f.Name())
	if err == nil {
		t.Error("expected error when override is a file, not a directory")
	}
}
