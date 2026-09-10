package gen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// TestGeneratedFilesAreUpToDate runs the generator on the schema snapshot and
// compares the result with the files checked in at the module root.
func TestGeneratedFilesAreUpToDate(t *testing.T) {
	root := filepath.Join("..", "..")
	files, err := Generate(
		readFile(t, filepath.Join(root, "schema", "herdr-api.schema.json")),
		readFile(t, filepath.Join(root, "schema", "method-results.json")),
		"herdr",
	)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, name := range Files {
		generated, ok := files[name]
		if !ok {
			t.Errorf("%s was not generated", name)
			continue
		}
		if want := readFile(t, filepath.Join(root, name)); !bytes.Equal(generated, want) {
			t.Errorf("%s differs from the generated output; run go generate ./...", name)
		}
	}
	if len(files) != len(Files) {
		t.Errorf("generated %d files, want %d", len(files), len(Files))
	}
}

// TestGenerateIsDeterministic guards the ordering of the emitted declarations.
func TestGenerateIsDeterministic(t *testing.T) {
	root := filepath.Join("..", "..")
	schema := readFile(t, filepath.Join(root, "schema", "herdr-api.schema.json"))
	methods := readFile(t, filepath.Join(root, "schema", "method-results.json"))
	first, err := Generate(schema, methods, "herdr")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	second, err := Generate(schema, methods, "herdr")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, name := range Files {
		if !bytes.Equal(first[name], second[name]) {
			t.Errorf("%s differs between two runs", name)
		}
	}
}
