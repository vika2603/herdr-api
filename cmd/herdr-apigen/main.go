// Command herdr-apigen generates the Go API surface of package herdr from
// the Herdr socket API schema. See docs/design.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vika2603/herdr-client/internal/gen"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "herdr-apigen:", err)
		os.Exit(1)
	}
}

func run() error {
	schemaPath := flag.String("schema", "schema/herdr-api.schema.json", "path to the Herdr API schema snapshot")
	methodsPath := flag.String("methods", "schema/method-results.json", "path to the method result table")
	outDir := flag.String("out", ".", "directory the generated files are written to")
	pkgName := flag.String("pkg", "herdr", "name of the generated package")
	flag.Parse()

	schemaData, err := os.ReadFile(*schemaPath)
	if err != nil {
		return err
	}
	methodsData, err := os.ReadFile(*methodsPath)
	if err != nil {
		return err
	}
	files, err := gen.Generate(schemaData, methodsData, *pkgName)
	if err != nil {
		return err
	}
	for _, name := range gen.Files {
		source, ok := files[name]
		if !ok {
			return fmt.Errorf("generator produced no %s", name)
		}
		if err := os.WriteFile(filepath.Join(*outDir, name), source, 0o644); err != nil {
			return err
		}
	}
	return nil
}
