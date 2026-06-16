// Package storage is a small blob store abstraction with a stdlib disk driver.
// No runtime dependency.
//
// It is OPTIONAL and self-configuring: build one with storage.FromEnv() (or
// storage.NewDisk(root)). Only the disk driver ships — to keep the dependency
// surface tiny, an S3/object-store driver is left as an extension point: implement
// the Storage interface (e.g. with a vendored minimal S3 client or the AWS SDK if
// you accept the dependency) and select it in FromEnv.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotFound is returned by Get when a key does not exist.
var ErrNotFound = errors.New("storage: not found")

// Storage stores opaque blobs under string keys (slash-separated paths).
// Implementations are safe for concurrent use.
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// FromEnv builds a Storage from env: STORAGE_DRIVER (disk, default) and
// STORAGE_DISK_ROOT (default ./storage).
func FromEnv() (Storage, error) {
	switch driver := getenv("STORAGE_DRIVER", "disk"); driver {
	case "disk":
		return NewDisk(getenv("STORAGE_DISK_ROOT", "./storage"))
	default:
		return nil, fmt.Errorf("storage: unknown STORAGE_DRIVER %q (built-in: disk; add others by implementing storage.Storage)", driver)
	}
}

// DiskStorage stores blobs as files under a root directory. Keys are sanitised so
// they can never escape the root (no path traversal).
type DiskStorage struct{ root string }

// NewDisk returns a DiskStorage rooted at dir, creating it if needed.
func NewDisk(dir string) (*DiskStorage, error) {
	root := filepath.Clean(dir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create root: %w", err)
	}
	return &DiskStorage{root: root}, nil
}

// resolve maps a key to an absolute path guaranteed to live under the root.
func (s *DiskStorage) resolve(key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", errors.New("storage: empty key")
	}
	// Leading "/" + Clean neutralises ".." segments before joining to the root.
	clean := filepath.Clean("/" + filepath.ToSlash(key))
	full := filepath.Join(s.root, clean)
	if full != s.root && !strings.HasPrefix(full, s.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("storage: key %q escapes root", key)
	}
	return full, nil
}

func (s *DiskStorage) Put(_ context.Context, key string, r io.Reader) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (s *DiskStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	full, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *DiskStorage) Delete(_ context.Context, key string) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *DiskStorage) Exists(_ context.Context, key string) (bool, error) {
	full, err := s.resolve(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(full)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
