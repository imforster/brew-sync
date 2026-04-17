package sync

import (
	"errors"
	"fmt"
	"os"
)

// FileBackend synchronizes the manifest by copying it to/from a shared filesystem path.
type FileBackend struct {
	RemotePath string // path to the shared filesystem location (file path)
}

// NewFileBackend creates a new FileBackend with the given remote file path.
func NewFileBackend(remotePath string) *FileBackend {
	return &FileBackend{
		RemotePath: remotePath,
	}
}

// Name returns "file".
func (f *FileBackend) Name() string {
	return "file"
}

// Pull copies the manifest from the remote shared filesystem path to dest.
// Returns a descriptive error if the remote file is not found or cannot be read.
func (f *FileBackend) Pull(dest string) error {
	if _, err := os.Stat(f.RemotePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("file backend: remote manifest not found at %s — verify the path is correct and the file has been pushed", f.RemotePath)
		}
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("file backend: permission denied reading %s — check file permissions", f.RemotePath)
		}
		return fmt.Errorf("file backend: cannot access remote manifest at %s: %w", f.RemotePath, err)
	}

	if err := copyFile(f.RemotePath, dest); err != nil {
		return fmt.Errorf("file backend: failed to pull manifest from %s to %s: %w", f.RemotePath, dest, err)
	}

	return nil
}

// Push copies the manifest from src to the remote shared filesystem path.
// Returns a descriptive error if the source file is missing or the destination is not writable.
func (f *FileBackend) Push(src string) error {
	if _, err := os.Stat(src); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("file backend: source manifest not found at %s", src)
		}
		return fmt.Errorf("file backend: cannot access source manifest at %s: %w", src, err)
	}

	if err := copyFile(src, f.RemotePath); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("file backend: permission denied writing to %s — check directory permissions", f.RemotePath)
		}
		return fmt.Errorf("file backend: failed to push manifest from %s to %s: %w", src, f.RemotePath, err)
	}

	return nil
}
