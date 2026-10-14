package wiring

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func TestCreationRedactsSecretsWithTheTraceMasker(t *testing.T) {
	s := &creation.Service{}
	WireCreationReferenceReads(s, &ingest.Service{}, &catalog.Service{})

	secret := "-----BEGIN " + "RSA PRIVATE KEY-----"
	if s.Mask == nil {
		t.Fatal("creation was wired without a masker; every page it fetches and every message it stores keeps its secrets")
	}
	if got := s.Mask("pasted: " + secret); strings.Contains(got, secret) {
		t.Errorf("Mask left the private key header in place: %q", got)
	}
}

func TestAReferenceWhoseCatalogFactsCannotBeReadIsMarkedUnknown(t *testing.T) {
	ref := creation.Reference{SkillID: "11111111-1111-4111-8111-111111111111", VersionID: "22222222-2222-4222-8222-222222222222"}

	addCatalogFacts(context.Background(), &catalog.Service{}, &ref)

	if ref.Tier != "unknown" || ref.ScanStatus != "unknown" || ref.Warnings != nil {
		t.Errorf("tier = %q scan = %q warnings = %v, want an explicit unknown with no warning count", ref.Tier, ref.ScanStatus, ref.Warnings)
	}
}

func TestOnlyAnUnusableReferenceReachesCreationAsNotFound(t *testing.T) {
	outage := errors.New("connection reset")
	for _, tc := range []struct {
		name         string
		err          error
		wantNotFound bool
	}{
		{"unusable", ingest.ErrReferenceUnavailable, true},
		{"malformed id", creation.ErrInvalidCommand, true},
		{"outage", outage, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := referenceReadError(tc.err)
			if errors.Is(got, creation.ErrNotFound) != tc.wantNotFound || !errors.Is(got, tc.err) {
				t.Errorf("err = %v, want not-found %v and the cause kept", got, tc.wantNotFound)
			}
		})
	}
}
