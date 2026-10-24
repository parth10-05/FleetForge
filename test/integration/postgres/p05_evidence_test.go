//go:build postgresintegration

package postgres

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const p05PostgresEvidenceDirectory = "FLEETFORGE_POSTGRES_EVIDENCE_DIR"

var p05PostgresEvidenceCases = []string{"P05-F17", "P05-F18", "P05-F19", "P05-F20", "P05-F21"}

type p05PostgresEvidenceReport struct {
	SchemaVersion string `json:"schema_version"`
	CaseID        string `json:"case_id"`
	SourceTest    string `json:"source_test"`
	Invariant     string `json:"invariant"`
	Result        string `json:"result"`
}

// recordP05PostgresEvidence retains a report only after the source test has
// completed successfully. It deliberately records no database identifiers or
// runtime values, because the database itself is not an artifact payload.
func recordP05PostgresEvidence(t *testing.T, caseID, invariant string) {
	t.Helper()
	directory := os.Getenv(p05PostgresEvidenceDirectory)
	if directory == "" {
		return
	}
	if !knownP05PostgresEvidenceCase(caseID) || invariant == "" {
		t.Fatalf("invalid P05 PostgreSQL evidence registration for %q", caseID)
	}
	source := t.Name()
	t.Cleanup(func() {
		if t.Failed() {
			return
		}
		if err := writeP05PostgresEvidenceReport(directory, p05PostgresEvidenceReport{
			SchemaVersion: "fleetforge.p05-postgres-evidence/v1", CaseID: caseID,
			SourceTest: source, Invariant: invariant, Result: "PASS",
		}); err != nil {
			t.Errorf("write P05 PostgreSQL evidence %s: %v", caseID, err)
		}
	})
}

func writeP05PostgresEvidenceReport(directory string, report p05PostgresEvidenceReport) error {
	if !knownP05PostgresEvidenceCase(report.CaseID) || report.SourceTest == "" || report.Invariant == "" || report.Result != "PASS" {
		return fmt.Errorf("invalid P05 PostgreSQL report for %q", report.CaseID)
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	final := filepath.Join(directory, strings.ToLower(report.CaseID)+"-postgres.json")
	if _, err := os.Stat(final); err == nil {
		return fmt.Errorf("duplicate report")
	} else if !os.IsNotExist(err) {
		return err
	}
	temporary := final + ".tmp"
	if err := os.WriteFile(temporary, append(encoded, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, final)
}

func writeP05PostgresEvidenceIndex(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	reports := make([]p05PostgresEvidenceReport, 0, len(p05PostgresEvidenceCases))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "-postgres.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return err
		}
		var report p05PostgresEvidenceReport
		if err := json.Unmarshal(raw, &report); err != nil {
			return err
		}
		if !knownP05PostgresEvidenceCase(report.CaseID) || report.Result != "PASS" {
			return fmt.Errorf("invalid report %q", entry.Name())
		}
		reports = append(reports, report)
	}
	if len(reports) != len(p05PostgresEvidenceCases) {
		return fmt.Errorf("report count=%d, want %d", len(reports), len(p05PostgresEvidenceCases))
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].CaseID < reports[j].CaseID })
	for index, report := range reports {
		if report.CaseID != p05PostgresEvidenceCases[index] {
			return fmt.Errorf("case[%d]=%s, want %s", index, report.CaseID, p05PostgresEvidenceCases[index])
		}
	}
	encoded, err := json.MarshalIndent(map[string]any{"schema_version": "fleetforge.p05-postgres-evidence-index/v1", "result": "PASS", "cases": reports}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "postgres-index.json"), append(encoded, '\n'), 0o600)
}

func knownP05PostgresEvidenceCase(caseID string) bool {
	for _, expected := range p05PostgresEvidenceCases {
		if caseID == expected {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	code := m.Run()
	if directory := os.Getenv(p05PostgresEvidenceDirectory); code == 0 && directory != "" {
		if err := writeP05PostgresEvidenceIndex(directory); err != nil {
			fmt.Fprintln(os.Stderr, "P05 PostgreSQL evidence index:", err)
			code = 1
		}
	}
	os.Exit(code)
}
