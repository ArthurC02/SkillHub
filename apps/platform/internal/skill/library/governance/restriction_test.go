package governance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

func TestRestrictionServiceRejectsInvalidInputBeforeDatabase(t *testing.T) {
	svc := &Service{}
	zeroUUID := pgtype.UUID{}
	tests := []struct {
		name string
		call func() error
		want string
	}{
		{"blank reason", func() error {
			_, err := svc.SetRestriction(context.Background(), zeroUUID, zeroUUID, "  ", "reviewed")
			return err
		}, "reason is required"},
		{"unknown reason", func() error {
			_, err := svc.SetRestriction(context.Background(), zeroUUID, zeroUUID, "invented", "reviewed")
			return err
		}, "unknown reason code"},
		{"blank set note", func() error {
			_, err := svc.SetRestriction(context.Background(), zeroUUID, zeroUUID, "license-review", "  ")
			return err
		}, "note is required"},
		{"long set note", func() error {
			_, err := svc.SetRestriction(context.Background(), zeroUUID, zeroUUID, "license-review", strings.Repeat("x", maxOperatorNoteChars+1))
			return err
		}, "note is too long"},
		{"blank clear note", func() error {
			_, err := svc.ClearRestriction(context.Background(), zeroUUID, zeroUUID, "  ")
			return err
		}, "note is required"},
		{"long clear note", func() error {
			_, err := svc.ClearRestriction(context.Background(), zeroUUID, zeroUUID, strings.Repeat("x", maxOperatorNoteChars+1))
			return err
		}, "note is too long"},
		{"blank redistribution", func() error {
			_, err := svc.SetRedistribution(context.Background(), zeroUUID, zeroUUID, RedistributionChange{Value: "  ", Note: "reviewed", Claim: registry.LicenseClaim{}})
			return err
		}, "value is required"},
		{"provenance redistribution", func() error {
			_, err := svc.SetRedistribution(context.Background(), zeroUUID, zeroUUID, RedistributionChange{Value: "self_supplied", Note: "reviewed", Claim: registry.LicenseClaim{}})
			return err
		}, "只有匯入流程能確立"},
		{"unknown redistribution is named before the note", func() error {
			_, err := svc.SetRedistribution(context.Background(), zeroUUID, zeroUUID, RedistributionChange{Value: "shared", Note: "  ", Claim: registry.LicenseClaim{}})
			return err
		}, "不認得的再散布判定"},
		{"release without licence evidence", func() error {
			_, err := svc.SetRedistribution(context.Background(), zeroUUID, zeroUUID, RedistributionChange{Value: "allowed", Note: "reviewed",
				Claim: registry.LicenseClaim{Expression: "MIT"}})
			return err
		}, "要附上依據的授權證據"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestALicenceMismatchNamesWhatTheNewestVersionRecords(t *testing.T) {
	err := redistributionRefusal(registry.Refused{
		Reason:   registry.RefusedLicenseMismatch,
		Recorded: registry.LicenseClaim{Expression: "MIT", Source: "LICENSE"},
	}, "allowed")

	var inputErr restrictionInputError
	if !errors.As(err, &inputErr) || !strings.Contains(err.Error(), "記錄的是 MIT，出處 LICENSE") {
		t.Fatalf("got %v, want an input error naming the recorded licence", err)
	}
}

func TestValidRestrictionNoteAcceptsExactlyTheByteCap(t *testing.T) {
	if _, err := validRestrictionNote(strings.Repeat("x", maxOperatorNoteChars)); err != nil {
		t.Errorf("a note at exactly the byte cap was rejected: %v", err)
	}
}

func TestARestrictionNoteIsCountedInCharacters(t *testing.T) {
	if _, err := validRestrictionNote(strings.Repeat("字", maxOperatorNoteChars)); err != nil {
		t.Errorf("a note of exactly the cap in Chinese characters was rejected: %v", err)
	}
	if _, err := validRestrictionNote(strings.Repeat("字", maxOperatorNoteChars+1)); err == nil {
		t.Error("a note one character over the cap was accepted")
	}
}
