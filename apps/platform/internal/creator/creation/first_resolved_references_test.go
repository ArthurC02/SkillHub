package creation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

func TestOnlyTheFirstResolvableReferencesUpToTheCapAreKept(t *testing.T) {
	var asked []string
	s := &Service{ResolveReference: func(_ context.Context, _ identity.Workspace, id, _ string) (Reference, ReferenceSkill, error) {
		asked = append(asked, id)
		if id == "gone" {
			return Reference{}, ReferenceSkill{}, errors.New("not readable")
		}
		return Reference{SkillID: id}, ReferenceSkill{}, nil
	}}

	refs := s.FirstResolvedReferences(context.Background(), identity.Workspace{}, []string{"a", "gone", "b", "c", "d"})

	var got []string
	for _, r := range refs {
		got = append(got, r.SkillID)
	}
	if len(got) != MaxReferences || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("references = %v, want the first %d that resolve: [a b c]", got, MaxReferences)
	}
	if len(asked) != 4 {
		t.Errorf("resolved %v, want resolution to stop once the cap is reached", asked)
	}
}

func TestOnlyAReferenceThatIsGoneIsReportedNotFound(t *testing.T) {
	outage := errors.New("connection reset")
	gone := fmt.Errorf("%w: the skill was unpublished", ErrNotFound)
	for _, tc := range []struct {
		name    string
		readErr error
		want    error
	}{
		{"the reference is gone", gone, ErrNotFound},
		{"the platform could not read it", outage, outage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolve := func(context.Context, identity.Workspace, string, string) (Reference, ReferenceSkill, error) {
				return Reference{}, ReferenceSkill{}, tc.readErr
			}
			read := func(context.Context, identity.Workspace, string, string) (ReferenceSkill, error) {
				return ReferenceSkill{}, tc.readErr
			}
			s := &Service{ResolveReference: resolve, ReadReferenceContent: read}
			ws := identity.Workspace{}
			confirmed := []Reference{{SkillID: "a", Confirmed: true}}

			_, contentErr := s.referencedContent(context.Background(), ws, confirmed)
			_, selectErr := s.selectReferences(context.Background(), ws, &Snapshot{}, Command{ReferenceSkillIDs: []string{"a"}})
			_, confirmErr := s.confirmReferences(context.Background(), ws, &Snapshot{PendingAction: PendingReferenceChoice, References: confirmed})
			saveErr := s.referencesResolve(context.Background(), ws, confirmed)

			for name, err := range map[string]error{"content": contentErr, "select": selectErr, "confirm": confirmErr, "save": saveErr} {
				if !errors.Is(err, tc.want) {
					t.Errorf("%s: err = %v, want %v", name, err, tc.want)
				}
				if errors.Is(tc.want, outage) && errors.Is(err, ErrNotFound) {
					t.Errorf("%s: an outage was reported as the reference being gone", name)
				}
			}
		})
	}
}
