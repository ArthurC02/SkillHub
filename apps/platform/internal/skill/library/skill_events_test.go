package registry

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
)

func TestEverySkillEventHasACatalogueNameAndAFixedPayloadShape(t *testing.T) {
	cases := []struct {
		event Event
		name  string
		keys  []string
	}{
		{SkillCreated{}, outbox.SkillCreated, []string{"forked_from_skill_id", "forked_from_version_id", "redistribution"}},
		{SkillVersionAdded{}, outbox.SkillVersionAdded, []string{"content_hash", "improved_by", "version_id", "version_number"}},
		{SkillDescribed{}, outbox.SkillDescribed, []string{}},
		{SkillTakenDown{}, outbox.SkillTakenDown, []string{}},
		{AccessRestricted{}, outbox.SkillAccessRestricted, []string{"reason"}},
		{AccessRestrictionLifted{}, outbox.SkillAccessRestrictionLifted, []string{}},
		{RedistributionSet{}, outbox.SkillRedistributionSet, []string{"after", "before"}},
		{SkillCategorized{}, outbox.SkillCategorized, []string{"category", "category_source"}},
		{SkillDeleted{}, outbox.SkillDeleted, []string{}},
	}
	for _, tc := range cases {
		if got := tc.event.eventType(); got != tc.name || !slices.Contains(outbox.EventTypes, got) {
			t.Errorf("%T is sent as %q, want %q from the closed set", tc.event, got, tc.name)
		}
		encoded, err := json.Marshal(tc.event)
		if err != nil {
			t.Fatalf("%T: %v", tc.event, err)
		}
		var payload map[string]any
		if err := json.Unmarshal(encoded, &payload); err != nil {
			t.Fatalf("%T: %v", tc.event, err)
		}
		keys := make([]string, 0, len(payload))
		for key := range payload {
			keys = append(keys, key)
		}
		if slices.Sort(keys); !slices.Equal(keys, tc.keys) {
			t.Errorf("%T payload keys = %v, want %v", tc.event, keys, tc.keys)
		}
	}
	if got := (Refused{}).eventType(); got != "" {
		t.Errorf("a refusal is an answer, not a fact to publish, yet it names %q", got)
	}
}
