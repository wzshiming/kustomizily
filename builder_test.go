package kustomizily

import (
	"bytes"
	"flag"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func processAndBuild(t *testing.T, inputPath string) map[string][]byte {
	t.Helper()
	f, err := os.Open(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	b := NewBuilder()
	if err := b.Process(f); err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	err = b.Build(func(dir string, name string, data []byte) error {
		out[path.Join(dir, name)] = append([]byte(nil), data...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGolden(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			caseDir := filepath.Join("testdata", e.Name())
			got := processAndBuild(t, filepath.Join(caseDir, "input.yaml"))
			goldenDir := filepath.Join(caseDir, "expected")

			if *update {
				if err := os.RemoveAll(goldenDir); err != nil {
					t.Fatal(err)
				}
				for name, data := range got {
					p := filepath.Join(goldenDir, filepath.FromSlash(name))
					if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(p, data, 0644); err != nil {
						t.Fatal(err)
					}
				}
				return
			}

			want := map[string][]byte{}
			err := filepath.WalkDir(goldenDir, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(goldenDir, p)
				if err != nil {
					return err
				}
				data, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				want[filepath.ToSlash(rel)] = data
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			for name := range want {
				if _, ok := got[name]; !ok {
					t.Errorf("missing file %s", name)
				}
			}
			for name, data := range got {
				wantData, ok := want[name]
				if !ok {
					t.Errorf("unexpected file %s", name)
					continue
				}
				if !bytes.Equal(data, wantData) {
					t.Errorf("file %s mismatch\n--- want ---\n%s\n--- got ---\n%s", name, wantData, data)
				}
			}
		})
	}
}

func TestBuildDeterministic(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "complete", "input.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var prev map[string][]byte
	for i := range 5 {
		b := NewBuilder()
		if err := b.Process(bytes.NewReader(input)); err != nil {
			t.Fatal(err)
		}
		got := map[string][]byte{}
		err := b.Build(func(dir string, name string, data []byte) error {
			got[path.Join(dir, name)] = append([]byte(nil), data...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if prev != nil && !maps.EqualFunc(prev, got, bytes.Equal) {
			t.Fatalf("run %d produced different output", i)
		}
		prev = got
	}
}

func TestProcessParseErrorContext(t *testing.T) {
	input := "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: a\n---\nfoo: [unclosed\n"
	err := NewBuilder().Process(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "document 1") {
		t.Fatalf("expected document index in error, got: %v", err)
	}
}

func TestProcessBase64ErrorContext(t *testing.T) {
	input := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: s\ndata:\n  token: '!!!'\n"
	err := NewBuilder().Process(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), `secret s: data key "token"`) {
		t.Fatalf("expected secret context in error, got: %v", err)
	}
}
