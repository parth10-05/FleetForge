//go:build postgresintegration

package v1demo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const p06EvidenceDirectory = "FLEETFORGE_P06_EVIDENCE_DIR"

var p06Cases = []string{"P06-A01", "P06-A02", "P06-A03", "P06-A04", "P06-A05"}

type p06EvidenceReport struct {
	SchemaVersion        string            `json:"schema_version"`
	CaseID               string            `json:"case_id"`
	CandidateSHA         string            `json:"candidate_sha"`
	MergeBaseSHA         string            `json:"merge_base_sha"`
	OpenAPIVersion       string            `json:"openapi_version"`
	MigrationVersion     string            `json:"migration_version"`
	SimulatorVersion     string            `json:"simulator_version"`
	IaCVersion           string            `json:"iac_version"`
	Command              string            `json:"command"`
	ExitStatus           int               `json:"exit_status"`
	RecordedAtUTC        time.Time         `json:"recorded_at_utc"`
	DurationMilliseconds int64             `json:"duration_milliseconds"`
	SeedSHA256           string            `json:"seed_sha256"`
	FixtureSHA256        string            `json:"fixture_sha256"`
	Environment          string            `json:"environment"`
	Invariant            string            `json:"invariant"`
	Counts               map[string]int64  `json:"counts"`
	Digests              map[string]string `json:"digests"`
	Limitations          []string          `json:"limitations"`
	Result               string            `json:"result"`
}

func recordP06Evidence(t *testing.T, caseID, seed, fixturePath, invariant string, counts map[string]int64, digests map[string]string) {
	t.Helper()
	directory := os.Getenv(p06EvidenceDirectory)
	if directory == "" {
		return
	}
	if !knownP06Case(caseID) || seed == "" || fixturePath == "" || invariant == "" {
		t.Fatalf("invalid P06 evidence registration")
	}
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read P06 fixture: %v", err)
	}
	seedDigest, fixtureDigest := sha256.Sum256([]byte(seed)), sha256.Sum256(fixture)
	report := p06EvidenceReport{SchemaVersion: "fleetforge.p06-acceptance-evidence/v1", CaseID: caseID, CandidateSHA: requiredEvidenceEnvironment(t, "FLEETFORGE_P06_CANDIDATE_SHA"), MergeBaseSHA: requiredEvidenceEnvironment(t, "FLEETFORGE_P06_MERGE_BASE_SHA"), OpenAPIVersion: "openapi/3.1", MigrationVersion: "00030", SimulatorVersion: "synthetic-redfish/v1", IaCVersion: "unchanged", Command: "go test -tags=postgresintegration -count=1 ./test/e2e/v1demo/...", ExitStatus: 0, RecordedAtUTC: time.Now().UTC(), SeedSHA256: "sha256:" + hex.EncodeToString(seedDigest[:]), FixtureSHA256: "sha256:" + hex.EncodeToString(fixtureDigest[:]), Environment: evidenceEnvironment(), Invariant: invariant, Counts: cloneCounts(counts), Digests: cloneDigests(digests), Limitations: []string{"local-synthetic-target-only"}, Result: "PASS"}
	t.Cleanup(func() {
		if !t.Failed() {
			if err := writeP06Evidence(directory, report); err != nil {
				t.Errorf("write P06 evidence %s: %v", caseID, err)
			}
		}
	})
}

func writeP06Evidence(directory string, report p06EvidenceReport) error {
	if !validP06Evidence(report) {
		return fmt.Errorf("invalid P06 evidence report")
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	final := filepath.Join(directory, strings.ToLower(report.CaseID)+".json")
	if _, err := os.Stat(final); err == nil {
		return fmt.Errorf("duplicate P06 evidence report")
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(final, append(encoded, '\n'), 0o600)
}

func writeP06EvidenceIndex(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	reports := make([]p06EvidenceReport, 0, len(p06Cases))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "p06-a") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return err
		}
		var report p06EvidenceReport
		if err := json.Unmarshal(raw, &report); err != nil || !validP06Evidence(report) {
			return fmt.Errorf("invalid P06 evidence %q", entry.Name())
		}
		reports = append(reports, report)
	}
	if len(reports) != len(p06Cases) {
		return fmt.Errorf("P06 evidence count=%d, want %d", len(reports), len(p06Cases))
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].CaseID < reports[j].CaseID })
	for index, report := range reports {
		if report.CaseID != p06Cases[index] {
			return fmt.Errorf("unexpected P06 evidence case %q", report.CaseID)
		}
	}
	checksums := make(map[string]string, len(reports))
	for _, report := range reports {
		raw, err := os.ReadFile(filepath.Join(directory, strings.ToLower(report.CaseID)+".json"))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		checksums[report.CaseID] = "sha256:" + hex.EncodeToString(sum[:])
	}
	encoded, err := json.MarshalIndent(map[string]any{"schema_version": "fleetforge.p06-acceptance-index/v1", "result": "PASS", "environment": evidenceEnvironment(), "reports": reports, "checksums": checksums}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "index.json"), append(encoded, '\n'), 0o600)
}

func validP06Evidence(report p06EvidenceReport) bool {
	if report.SchemaVersion != "fleetforge.p06-acceptance-evidence/v1" || !knownP06Case(report.CaseID) || report.Result != "PASS" || report.Invariant == "" || report.Command == "" || report.ExitStatus != 0 || report.RecordedAtUTC.IsZero() || report.DurationMilliseconds < 0 || !commitSHA(report.CandidateSHA) || !commitSHA(report.MergeBaseSHA) || report.OpenAPIVersion == "" || report.MigrationVersion == "" || report.SimulatorVersion == "" || report.IaCVersion == "" || len(report.Limitations) == 0 || (report.Environment != "local-working-tree" && report.Environment != "pull-request-hosted" && report.Environment != "main-hosted") || !digest(report.SeedSHA256) || !digest(report.FixtureSHA256) {
		return false
	}
	encoded, err := json.Marshal(report)
	if err != nil || containsForbiddenEvidence(string(encoded)) {
		return false
	}
	for key, value := range report.Digests {
		if key == "" || !digest(value) {
			return false
		}
	}
	return true
}
func commitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
func containsForbiddenEvidence(value string) bool {
	lower := strings.ToLower(value)
	for _, forbidden := range []string{"postgres://", "postgresql://", "bearer ", "\"password\"", "\"credential", "\"authorization\"", "\"token\"", "signed_url", "callback_payload", "request_body", "response_body"} {
		if strings.Contains(lower, forbidden) {
			return true
		}
	}
	return false
}
func requiredEvidenceEnvironment(t *testing.T, key string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if !commitSHA(value) {
		t.Fatalf("%s must contain a full lowercase commit SHA when P06 evidence is enabled", key)
	}
	return value
}
func digest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && len(value) == len("sha256:")+64
}
func knownP06Case(value string) bool {
	for _, expected := range p06Cases {
		if value == expected {
			return true
		}
	}
	return false
}
func evidenceEnvironment() string {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		if os.Getenv("GITHUB_REF_NAME") == "main" {
			return "main-hosted"
		}
		return "pull-request-hosted"
	}
	return "local-working-tree"
}
func cloneCounts(source map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
func cloneDigests(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func TestMain(m *testing.M) {
	code := m.Run()
	if directory := os.Getenv(p06EvidenceDirectory); code == 0 && directory != "" {
		if err := writeP06EvidenceIndex(directory); err != nil {
			fmt.Fprintln(os.Stderr, "P06 evidence index:", err)
			code = 1
		}
	}
	os.Exit(code)
}

func TestP06EvidenceIndexRequiresEveryCaseOnce(t *testing.T) {
	directory := t.TempDir()
	for _, caseID := range p06Cases {
		report := testP06EvidenceReport(caseID)
		if err := writeP06Evidence(directory, report); err != nil {
			t.Fatalf("write %s: %v", caseID, err)
		}
	}
	if err := writeP06EvidenceIndex(directory); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "index.json")); err != nil {
		t.Fatalf("index missing: %v", err)
	}
}

func TestP06EvidenceIndexRejectsIncompleteCases(t *testing.T) {
	directory := t.TempDir()
	report := testP06EvidenceReport(p06Cases[0])
	if err := writeP06Evidence(directory, report); err != nil {
		t.Fatal(err)
	}
	if err := writeP06EvidenceIndex(directory); err == nil {
		t.Fatal("incomplete evidence index succeeded")
	}
}

func TestP06EvidenceRejectsSecretBearingValues(t *testing.T) {
	report := testP06EvidenceReport("P06-A01")
	report.Invariant = "authorization: Bearer token"
	if validP06Evidence(report) {
		t.Fatal("secret-bearing evidence was accepted")
	}
}

func testP06EvidenceReport(caseID string) p06EvidenceReport {
	return p06EvidenceReport{SchemaVersion: "fleetforge.p06-acceptance-evidence/v1", CaseID: caseID, CandidateSHA: strings.Repeat("a", 40), MergeBaseSHA: strings.Repeat("b", 40), OpenAPIVersion: "openapi/3.1", MigrationVersion: "00030", SimulatorVersion: "synthetic-redfish/v1", IaCVersion: "unchanged", Command: "go test", ExitStatus: 0, RecordedAtUTC: time.Now().UTC(), SeedSHA256: "sha256:" + strings.Repeat("a", 64), FixtureSHA256: "sha256:" + strings.Repeat("b", 64), Environment: "local-working-tree", Invariant: "durable invariant", Counts: map[string]int64{"audit_events": 1}, Digests: map[string]string{"journal": "sha256:" + strings.Repeat("c", 64)}, Limitations: []string{"local-synthetic-target-only"}, Result: "PASS"}
}
