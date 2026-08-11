package kustomizily

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePath(t *testing.T) {
	valid := []struct{ dir, name string }{
		{"", "kustomization.yaml"},
		{"crd", "a.yaml"},
		{"a/b", "x"},
	}
	for _, tt := range valid {
		if err := validatePath(tt.dir, tt.name); err != nil {
			t.Errorf("validatePath(%q, %q) = %v, want nil", tt.dir, tt.name, err)
		}
	}

	invalid := []struct{ dir, name string }{
		{"", ""},
		{"", "."},
		{"", ".."},
		{"", "a/b.yaml"},
		{"", `a\b.yaml`},
		{"..", "a.yaml"},
		{"../x", "a.yaml"},
		{"a/../b", "a.yaml"},
		{"/abs", "a.yaml"},
		{`a\b`, "a.yaml"},
		{"a//b", "a.yaml"},
	}
	for _, tt := range invalid {
		if err := validatePath(tt.dir, tt.name); err == nil {
			t.Errorf("validatePath(%q, %q) = nil, want error", tt.dir, tt.name)
		}
	}
}

func TestFSWriteFile(t *testing.T) {
	root := t.TempDir()
	f := NewFS(root)

	if err := f.WriteFile("sub", "a.yaml", []byte("x")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "sub", "a.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "x" {
		t.Errorf("got %q, want %q", data, "x")
	}

	if err := f.WriteFile("../evil", "a.yaml", []byte("x")); err == nil {
		t.Error("expected error for traversal dir")
	}
	if err := f.WriteFile("", "../evil.yaml", []byte("x")); err == nil {
		t.Error("expected error for traversal name")
	}
}
