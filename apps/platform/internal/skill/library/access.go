package registry

import "strings"

type AccessRestriction struct {
	reason string
}

func RestrictionFrom(recorded *string) AccessRestriction {
	if recorded == nil {
		return AccessRestriction{}
	}
	return AccessRestriction{reason: strings.TrimSpace(*recorded)}
}

func (r AccessRestriction) InEffect() bool { return r.reason != "" }

func (r AccessRestriction) Reason() string { return r.reason }

func (s Skill) Restriction() AccessRestriction { return RestrictionFrom(s.AccessRestriction) }

func (v VersionSummary) Restriction() AccessRestriction { return RestrictionFrom(v.AccessRestriction) }
