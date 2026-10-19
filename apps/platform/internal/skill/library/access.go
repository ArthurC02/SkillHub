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

func (s Skill) Restriction() AccessRestriction {
	return ownOrInherited(s.AccessRestriction, s.InheritedRestriction)
}

func (v VersionSummary) Restriction() AccessRestriction {
	return ownOrInherited(v.AccessRestriction, v.InheritedRestriction)
}

func ownOrInherited(own *string, inherited string) AccessRestriction {
	if r := RestrictionFrom(own); r.InEffect() {
		return r
	}
	return RestrictionFrom(&inherited)
}
