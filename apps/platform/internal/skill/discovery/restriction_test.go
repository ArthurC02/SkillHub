package catalog

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
			_, err := svc.SetRestriction(context.Background(), zeroUUID, zeroUUID, "license-review", strings.Repeat("x", maxOperatorNoteBytes+1))
			return err
		}, "note is too long"},
		{"blank clear note", func() error {
			_, err := svc.ClearRestriction(context.Background(), zeroUUID, zeroUUID, "  ")
			return err
		}, "note is required"},
		{"long clear note", func() error {
			_, err := svc.ClearRestriction(context.Background(), zeroUUID, zeroUUID, strings.Repeat("x", maxOperatorNoteBytes+1))
			return err
		}, "note is too long"},
		{"blank redistribution", func() error {
			_, err := svc.SetRedistribution(context.Background(), zeroUUID, zeroUUID, "  ", "reviewed", registry.LicenseClaim{})
			return err
		}, "value is required"},
		{"provenance redistribution", func() error {
			_, err := svc.SetRedistribution(context.Background(), zeroUUID, zeroUUID, "self_supplied", "reviewed", registry.LicenseClaim{})
			return err
		}, "only the import path can establish that"},
		{"unknown redistribution is named before the note", func() error {
			_, err := svc.SetRedistribution(context.Background(), zeroUUID, zeroUUID, "shared", "  ", registry.LicenseClaim{})
			return err
		}, "unknown redistribution value"},
		{"release without licence evidence", func() error {
			_, err := svc.SetRedistribution(context.Background(), zeroUUID, zeroUUID, "allowed", "reviewed",
				registry.LicenseClaim{Expression: "MIT"})
			return err
		}, "requires the licence evidence"},
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
	if !errors.As(err, &inputErr) || !strings.Contains(err.Error(), "recorded: MIT from LICENSE") {
		t.Fatalf("got %v, want an input error naming the recorded licence", err)
	}
}

func TestValidRestrictionNoteAcceptsExactlyTheByteCap(t *testing.T) {
	if _, err := validRestrictionNote(strings.Repeat("x", maxOperatorNoteBytes)); err != nil {
		t.Errorf("a note at exactly the byte cap was rejected: %v", err)
	}
}
