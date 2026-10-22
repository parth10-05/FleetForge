//go:build postgresintegration

package fault

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
)

const faultEvidenceDirectory = "FLEETFORGE_FAULT_EVIDENCE_DIR"

var composedEvidenceCases = []string{
	"P05-F01", "P05-F02", "P05-F03", "P05-F04", "P05-F05", "P05-F06",
	"P05-F07", "P05-F08", "P05-F09", "P05-F10", "P05-F11", "P05-F12",
	"P05-F13", "P05-F14", "P05-F15", "P05-F16",
}

type faultEvidenceReport struct {
	SchemaVersion  string `json:"schema_version"`
	CaseID         string `json:"case_id"`
	ScenarioSHA256 string `json:"scenario_sha256"`
	Invariant      string `json:"invariant"`
	Result         string `json:"result"`
}

// recordFaultEvidence registers a write only after the owning test has
// succeeded. This keeps a failed oracle from being retained as a passed case.
func recordFaultEvidence(t *testing.T, scenarioPath, invariant string, caseIDs ...string) {
	t.Helper()
	directory := os.Getenv(faultEvidenceDirectory)
	if directory == "" {
		return
	}
	if scenarioPath == "" || invariant == "" || len(caseIDs) == 0 {
		t.Fatalf("invalid fault evidence registration")
	}
	raw, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatalf("read fault evidence scenario: %v", err)
	}
	digest := sha256.Sum256(raw)
	scenarioDigest := "sha256:" + hex.EncodeToString(digest[:])
	t.Cleanup(func() {
		if t.Failed() {
			return
		}
		for _, caseID := range caseIDs {
			if err := writeFaultEvidenceReport(directory, faultEvidenceReport{
				SchemaVersion: "fleetforge.p05-fault-evidence/v1",
				CaseID:        caseID, ScenarioSHA256: scenarioDigest, Invariant: invariant, Result: "PASS",
			}); err != nil {
				t.Errorf("write fault evidence %s: %v", caseID, err)
			}
		}
	})
}

func writeFaultEvidenceReport(directory string, report faultEvidenceReport) error {
	if !knownComposedEvidenceCase(report.CaseID) || report.Invariant == "" || report.Result != "PASS" || !strings.HasPrefix(report.ScenarioSHA256, "sha256:") || len(report.ScenarioSHA256) != len("sha256:")+64 {
		return fmt.Errorf("invalid redacted fault report for %q", report.CaseID)
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	name := strings.ToLower(report.CaseID) + ".json"
	final := filepath.Join(directory, name)
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

func writeFaultEvidenceIndex(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	reports := make([]faultEvidenceReport, 0, len(composedEvidenceCases))
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "index.json" || strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return err
		}
		var report faultEvidenceReport
		if err := json.Unmarshal(raw, &report); err != nil {
			return err
		}
		if !knownComposedEvidenceCase(report.CaseID) || report.Result != "PASS" {
			return fmt.Errorf("invalid report %q", entry.Name())
		}
		reports = append(reports, report)
	}
	if len(reports) != len(composedEvidenceCases) {
		return fmt.Errorf("report count=%d, want %d", len(reports), len(composedEvidenceCases))
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].CaseID < reports[j].CaseID })
	for index, report := range reports {
		if report.CaseID != composedEvidenceCases[index] {
			return fmt.Errorf("case[%d]=%s, want %s", index, report.CaseID, composedEvidenceCases[index])
		}
	}
	encoded, err := json.MarshalIndent(map[string]any{
		"schema_version": "fleetforge.p05-fault-evidence-index/v1",
		"result":         "PASS", "cases": reports,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "index.json"), append(encoded, '\n'), 0o600)
}

func knownComposedEvidenceCase(caseID string) bool {
	for _, expected := range composedEvidenceCases {
		if caseID == expected {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	code := m.Run()
	if directory := os.Getenv(faultEvidenceDirectory); code == 0 && directory != "" {
		if err := writeFaultEvidenceIndex(directory); err != nil {
			fmt.Fprintln(os.Stderr, "fault evidence index:", err)
			code = 1
		}
	}
	os.Exit(code)
}
