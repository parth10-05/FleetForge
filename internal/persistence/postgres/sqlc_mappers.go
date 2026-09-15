package postgres

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres/sqlcgen"
)

// CampaignRowFromSQLC translates generated persistence data into the strict
// row shape consumed by the domain-facing mapper helpers. It validates every
// nullable and vocabulary-bearing value before a repository can expose it.
func CampaignRowFromSQLC(row sqlcgen.Campaign) (CampaignRow, error) {
	id, err := sqlcUUIDText(row.ID)
	if err != nil {
		return CampaignRow{}, fmt.Errorf("campaign id: %w", err)
	}
	artifactID, err := sqlcUUIDText(row.ArtifactID)
	if err != nil {
		return CampaignRow{}, fmt.Errorf("campaign artifact id: %w", err)
	}
	policyVersionID, err := sqlcUUIDText(row.PolicyVersionID)
	if err != nil {
		return CampaignRow{}, fmt.Errorf("campaign policy version id: %w", err)
	}
	suspended, err := sqlcText(row.SuspendedPhase)
	if err != nil {
		return CampaignRow{}, fmt.Errorf("campaign suspended phase: %w", err)
	}
	frozenPolicy, err := sqlcText(row.FrozenPolicyHash)
	if err != nil {
		return CampaignRow{}, fmt.Errorf("campaign frozen policy hash: %w", err)
	}
	frozenTargets, err := sqlcText(row.FrozenTargetSnapshotHash)
	if err != nil {
		return CampaignRow{}, fmt.Errorf("campaign frozen target snapshot hash: %w", err)
	}
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return CampaignRow{}, fmt.Errorf("%w: campaign timestamp is null", ErrCorruptPersistedValue)
	}
	if _, err := CampaignRowState(row.State); err != nil {
		return CampaignRow{}, err
	}
	if _, err := CampaignRowSuspendedPhase(suspended); err != nil {
		return CampaignRow{}, err
	}
	return CampaignRow{
		ID: id, ArtifactID: artifactID, State: row.State, Revision: row.Revision,
		CommandEpoch: row.CommandEpoch, SuspendedPhase: suspended, PolicyVersionID: policyVersionID,
		FrozenPolicyHash: frozenPolicy, FrozenTargetSnapshotHash: frozenTargets,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

// TargetUpdateRowFromSQLC is the generated-row counterpart for target updates.
func TargetUpdateRowFromSQLC(row sqlcgen.TargetUpdate) (TargetUpdateRow, error) {
	id, err := sqlcUUIDText(row.ID)
	if err != nil {
		return TargetUpdateRow{}, fmt.Errorf("target update id: %w", err)
	}
	campaignTargetID, err := sqlcUUIDText(row.CampaignTargetID)
	if err != nil {
		return TargetUpdateRow{}, fmt.Errorf("target update campaign target id: %w", err)
	}
	resume, err := sqlcText(row.ResumeStep)
	if err != nil {
		return TargetUpdateRow{}, fmt.Errorf("target update resume step: %w", err)
	}
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return TargetUpdateRow{}, fmt.Errorf("%w: target update timestamp is null", ErrCorruptPersistedValue)
	}
	if _, err := TargetUpdateRowState(row.State); err != nil {
		return TargetUpdateRow{}, err
	}
	if _, err := TargetUpdateRowResumeStep(resume); err != nil {
		return TargetUpdateRow{}, err
	}
	return TargetUpdateRow{
		ID: id, CampaignTargetID: campaignTargetID, State: row.State, Revision: row.Revision,
		CommandEpoch: row.CommandEpoch, ResumeStep: resume, ExpectedFirmwareVersion: row.ExpectedFirmwareVersion,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func sqlcUUIDText(value pgtype.UUID) (string, error) {
	if !value.Valid {
		return "", fmt.Errorf("%w: null UUID", ErrCorruptPersistedValue)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", value.Bytes[0:4], value.Bytes[4:6], value.Bytes[6:8], value.Bytes[8:10], value.Bytes[10:16]), nil
}

func sqlcText(value pgtype.Text) (*string, error) {
	if !value.Valid {
		return nil, nil
	}
	copy := value.String
	return &copy, nil
}
