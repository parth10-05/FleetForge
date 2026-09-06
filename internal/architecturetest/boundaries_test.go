package architecturetest

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDomainDoesNotImportInfrastructure(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	domainPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "domain"))
	packages, err := parser.ParseDir(token.NewFileSet(), domainPath, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse domain package: %v", err)
	}
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, imported := range file.Imports {
				path := imported.Path.Value
				switch path {
				case "\"net/http\"", "\"database/sql\"", "\"go.opentelemetry.io/otel\"":
					t.Fatalf("domain imports forbidden infrastructure package %s", path)
				}
			}
		}
	}
}
