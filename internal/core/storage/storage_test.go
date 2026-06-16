package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestDiskRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, err := NewDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if ok, _ := s.Exists(ctx, "docs/a.txt"); ok {
		t.Error("Exists on missing key returned true")
	}
	if err := s.Put(ctx, "docs/a.txt", bytes.NewReader([]byte("hello"))); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Exists(ctx, "docs/a.txt"); err != nil || !ok {
		t.Fatalf("Exists = (%v, %v), want (true, nil)", ok, err)
	}

	rc, err := s.Get(ctx, "docs/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != "hello" {
		t.Errorf("Get = %q, want hello", got)
	}

	if err := s.Delete(ctx, "docs/a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "docs/a.txt"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete = %v, want ErrNotFound", err)
	}
}

func TestGetMissingIsNotFound(t *testing.T) {
	s, _ := NewDisk(t.TempDir())
	if _, err := s.Get(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing = %v, want ErrNotFound", err)
	}
}

// TestNoPathTraversal confirms a ".." key cannot write outside the root.
func TestNoPathTraversal(t *testing.T) {
	root := t.TempDir()
	s, _ := NewDisk(root)

	if err := s.Put(context.Background(), "../escape.txt", bytes.NewReader([]byte("x"))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Nothing must appear in the parent of root.
	if _, err := os.Stat(filepath.Join(root, "..", "escape.txt")); err == nil {
		t.Fatal("path traversal: file written outside the storage root")
	}
	// It is safely stored under the root instead, reachable by the same key.
	if ok, _ := s.Exists(context.Background(), "../escape.txt"); !ok {
		t.Error("sanitised key should still be retrievable")
	}
}
