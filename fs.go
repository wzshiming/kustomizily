package kustomizily

import (
	"fmt"
	"os"
	"path"
)

// FS implements a file system writer that creates directories and files on disk.
type FS struct {
	root string
	dirs map[string]struct{}
}

// NewFS creates a new file system writer with the specified root directory.
func NewFS(root string) *FS {
	return &FS{root: root, dirs: map[string]struct{}{}}
}

// WriteFile writes data to a file in the specified directory under the FS root.
func (f *FS) WriteFile(dir string, name string, data []byte) error {
	if _, ok := f.dirs[dir]; !ok {
		f.dirs[dir] = struct{}{}
		if err := os.MkdirAll(path.Join(f.root, dir), 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path.Join(f.root, dir, name), data, 0644)
}

// DryRunFS implements a file system writer that simulates file operations,
// printing actions to stdout instead of performing real disk operations.
// Useful for previewing changes without modifying the filesystem.
type DryRunFS struct {
	root string
	dirs map[string]struct{}
}

// NewDryRunFS creates a new dry-run file system writer with the specified root.
func NewDryRunFS(root string) *DryRunFS {
	return &DryRunFS{root: root, dirs: map[string]struct{}{}}
}

// WriteFile logs the file creation operation to stdout without writing to disk.
func (d *DryRunFS) WriteFile(dir string, name string, data []byte) error {
	if _, ok := d.dirs[dir]; !ok {
		d.dirs[dir] = struct{}{}
		fmt.Println("mkdir", path.Join(d.root, dir))
	}
	fmt.Println("write", path.Join(d.root, dir, name))
	return nil
}
