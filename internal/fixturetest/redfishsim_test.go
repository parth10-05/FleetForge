package fixturetest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixturePrefix = "/virtual-bmcs/alpha-001/redfish/v1/"

func TestRedfishFixtureContract(t *testing.T) {
	root := repositoryRoot(t)
	manifestPath := filepath.Join(root, "test", "fixtures", "redfishsim", "v1", "schema-manifest.json")
	var manifest struct {
		Source struct {
			Commit  string `json:"commit"`
			License string `json:"license"`
		} `json:"source"`
		Schemas []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"schemas"`
	}
	decodeJSON(t, manifestPath, &manifest)
	if manifest.Source.Commit != "4f81814e399213c9055863ddaff42791c6b3706a" || manifest.Source.License != "BSD-3-Clause" {
		t.Fatalf("manifest does not pin the accepted immutable DMTF source")
	}
	knownSchemas := make(map[string]bool, len(manifest.Schemas))
	for _, schema := range manifest.Schemas {
		if !strings.HasPrefix(schema.Path, "json-schema/") || len(schema.SHA256) != 64 {
			t.Fatalf("invalid selected schema record: %#v", schema)
		}
		knownSchemas[filepath.Base(schema.Path)] = true
	}

	fixtureDirectory := filepath.Join(root, "test", "fixtures", "redfishsim", "v1")
	entries, err := os.ReadDir(fixtureDirectory)
	if err != nil {
		t.Fatal(err)
	}
	routes := map[string]bool{}
	errors := map[int]bool{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "schema-manifest.json" {
			continue
		}
		path := filepath.Join(fixtureDirectory, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(raw))
		for _, forbidden := range []string{"password", "authorization", "basic "} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("fixture %s contains credential material", entry.Name())
			}
		}
		var fixture struct {
			Route       string                                            `json:"route"`
			Method      string                                            `json:"method"`
			Status      int                                               `json:"status"`
			Schema      string                                            `json:"schema"`
			NonMutating bool                                              `json:"nonMutating"`
			Fields      struct{ Standard, Extension, AdminOnly []string } `json:"fields"`
			Body        any                                               `json:"body"`
		}
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatalf("fixture %s is not JSON: %v", entry.Name(), err)
		}
		if fixture.Method == "" || fixture.Status < 200 || fixture.Status > 599 || fixture.Schema == "" || fixture.Body == nil {
			t.Fatalf("fixture %s is incomplete", entry.Name())
		}
		if !knownSchemas[fixture.Schema] {
			t.Fatalf("fixture %s references an unpinned schema %q", entry.Name(), fixture.Schema)
		}
		if fixture.NonMutating {
			errors[fixture.Status] = true
		}
		if fixture.Route == "*" {
			if !fixture.NonMutating {
				t.Fatalf("error fixture %s must declare nonMutating", entry.Name())
			}
		} else {
			if !strings.HasPrefix(fixture.Route, fixturePrefix) || strings.Contains(fixture.Route, "?") {
				t.Fatalf("fixture %s has non-canonical route %q", entry.Name(), fixture.Route)
			}
			routes[fixture.Method+" "+fixture.Route] = true
			assertFixtureURIs(t, entry.Name(), fixture.Body)
		}
		if len(fixture.Fields.Standard)+len(fixture.Fields.Extension)+len(fixture.Fields.AdminOnly) == 0 {
			t.Fatalf("fixture %s does not classify fields", entry.Name())
		}
	}
	for _, route := range []string{
		"GET " + fixturePrefix, "GET " + fixturePrefix + "Systems", "GET " + fixturePrefix + "Managers", "GET " + fixturePrefix + "Chassis",
		"GET " + fixturePrefix + "UpdateService", "POST " + fixturePrefix + "UpdateService/update", "GET " + fixturePrefix + "TaskService",
		"GET " + fixturePrefix + "EventService", "POST " + fixturePrefix + "EventService/Subscriptions",
	} {
		if !routes[route] {
			t.Errorf("missing canonical fixture for %s", route)
		}
	}
	for _, status := range []int{400, 401, 404, 405, 413, 415} {
		if !errors[status] {
			t.Errorf("missing bounded non-mutating error fixture for %d", status)
		}
	}
}

func assertFixtureURIs(t *testing.T, name string, value any) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		for key, nested := range value {
			if key == "@odata.id" {
				if uri, ok := nested.(string); !ok || !strings.HasPrefix(uri, fixturePrefix) {
					t.Fatalf("fixture %s has an escaped URI %v", name, nested)
				}
			}
			assertFixtureURIs(t, name, nested)
		}
	case []any:
		for _, nested := range value {
			assertFixtureURIs(t, name, nested)
		}
	}
}

func decodeJSON(t *testing.T, path string, destination any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		t.Fatalf("%s is not valid JSON: %v", path, err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("repository root not found")
		}
		directory = parent
	}
}
