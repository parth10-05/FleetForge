package architecturetest

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const domainImportPrefix = "github.com/parth10-05/fleetforge/internal/domain/"

var allowedDomainImports = map[string]map[string]struct{}{
	"domain":       {},
	"value":        {},
	"reason":       {},
	"evidence":     {"value": {}},
	"artifact":     {"value": {}},
	"inventory":    {"value": {}},
	"policy":       {"value": {}},
	"canary":       {"value": {}},
	"campaign":     {"value": {}, "reason": {}, "policy": {}},
	"wave":         {"value": {}, "reason": {}},
	"targetupdate": {"value": {}, "reason": {}, "evidence": {}, "policy": {}},
	"event":        {"value": {}, "reason": {}},
	"replay":       {"campaign": {}, "targetupdate": {}, "event": {}},
}

var forbiddenStandardImports = map[string]struct{}{
	"database/sql": {},
	"log":          {},
	"log/slog":     {},
	"math/rand":    {},
	"net/http":     {},
	"os":           {},
}

func TestDomainDoesNotImportInfrastructure(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	domainPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "domain"))
	err := filepath.WalkDir(domainPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relativePath, err := filepath.Rel(domainPath, path)
		if err != nil {
			return fmt.Errorf("resolve domain path %q: %w", path, err)
		}
		packageName := filepath.Dir(relativePath)
		if packageName == "." {
			packageName = "domain"
		}
		if _, known := allowedDomainImports[packageName]; !known {
			return fmt.Errorf("domain package %q has no declared import policy", packageName)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %q: %w", path, err)
		}
		for _, imported := range parsed.Imports {
			if err := validateDomainImport(packageName, strings.Trim(imported.Path.Value, "\"")); err != nil {
				return fmt.Errorf("%s: %w", relativePath, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("domain dependency boundary: %v", err)
	}
}

func validateDomainImport(packageName, imported string) error {
	if _, known := allowedDomainImports[packageName]; !known {
		return fmt.Errorf("domain package %q has no declared import policy", packageName)
	}
	if _, forbidden := forbiddenStandardImports[imported]; forbidden {
		return fmt.Errorf("forbidden infrastructure import %q", imported)
	}
	if strings.HasPrefix(imported, domainImportPrefix) {
		dependency := strings.TrimPrefix(imported, domainImportPrefix)
		if _, allowed := allowedDomainImports[packageName][dependency]; !allowed {
			return fmt.Errorf("package %q may not import domain package %q", packageName, dependency)
		}
		return nil
	}
	if strings.Contains(imported, ".") {
		return fmt.Errorf("external dependency %q is not allowed in production domain code", imported)
	}
	return nil
}

func TestDomainImportPolicy(t *testing.T) {
	testCases := []struct {
		name        string
		packageName string
		imported    string
		wantError   bool
	}{
		{name: "standard library", packageName: "value", imported: "strings"},
		{name: "allowed inward dependency", packageName: "campaign", imported: domainImportPrefix + "value"},
		{name: "forbidden standard library infrastructure", packageName: "campaign", imported: "net/http", wantError: true},
		{name: "forbidden external dependency", packageName: "value", imported: "github.com/google/uuid", wantError: true},
		{name: "forbidden outward domain dependency", packageName: "replay", imported: domainImportPrefix + "value", wantError: true},
		{name: "unknown domain package", packageName: "unknown", imported: "strings", wantError: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateDomainImport(testCase.packageName, testCase.imported)
			if (err != nil) != testCase.wantError {
				t.Fatalf("validateDomainImport(%q, %q) error = %v, want error %t", testCase.packageName, testCase.imported, err, testCase.wantError)
			}
		})
	}
}
