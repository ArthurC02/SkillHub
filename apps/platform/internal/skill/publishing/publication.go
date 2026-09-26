package publishing

import "errors"

type Status string

const (
	StatusPublished Status = "published"
	StatusDelisted  Status = "delisted"
)

func AllStatuses() []Status {
	return []Status{StatusPublished, StatusDelisted}
}

type Availability string

const (
	AvailabilityAvailable        Availability = "available"
	AvailabilityDelisted         Availability = "delisted"
	AvailabilityWithdrawn        Availability = "withdrawn"
	AvailabilityTakenDown        Availability = "taken_down"
	AvailabilityHeld             Availability = "held"
	AvailabilityNotRedistributed Availability = "not_redistributable"
)

type Refusal string

const (
	RefusedLicenseHold        Refusal = "license_hold"
	RefusedNotRedistributable Refusal = "not_redistributable"
	RefusedLicenseUnknown     Refusal = "license_unknown"
	RefusedValidation         Refusal = "validation_blocked"
	RefusedRightsNotAttested  Refusal = "rights_not_attested"
	RefusedFileRemoved        Refusal = "file_removed_by_packager"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrNoPublisher     = errors.New("this account has no publisher name yet; register one before publishing")
	ErrPublisherExists = errors.New("this account already has a publisher name")
	ErrNameTaken       = errors.New("that name is already taken")
	ErrNameIsPermanent = errors.New("this Skill is already published under another name, and names cannot change")
)

type NameError struct {
	Problem NameProblem
}

func (e *NameError) Error() string {
	if e.Problem == NameReserved {
		return "that name is reserved"
	}
	return "a name is 1 to 64 lowercase letters, digits and single hyphens, starting and ending with a letter or digit"
}

type RefusedError struct {
	Reason  Refusal
	Message string
}

func (e *RefusedError) Error() string { return e.Message }

type UnavailableError struct {
	Availability Availability
}

func (e *UnavailableError) Error() string {
	return "this publication is not offered: " + string(e.Availability)
}

const (
	redistributionAllowed      = "allowed"
	redistributionBlocked      = "blocked"
	redistributionSelfSupplied = "self_supplied"
	redistributionGenerated    = "generated"
)

func releaseGate(skill SkillFacts, rightsAttested bool) *RefusedError {
	if skill.AccessRestricted {
		return &RefusedError{RefusedLicenseHold, "這個 Skill 的內容因授權問題尚未釐清而被保留，所以不能發佈"}
	}
	switch skill.Redistribution {
	case redistributionAllowed:
		return nil
	case redistributionSelfSupplied, redistributionGenerated:
		if !rightsAttested {
			return &RefusedError{RefusedRightsNotAttested,
				"這份內容是你自己帶進來的，或是平台依你的描述寫出來的；發佈之前要先聲明你有權散布它"}
		}
		return nil
	case redistributionBlocked:
		return &RefusedError{RefusedNotRedistributable, "這個 Skill 的授權不允許再散布，所以不能發佈"}
	default:
		return &RefusedError{RefusedLicenseUnknown, "沒有人確認過這個 Skill 可不可以再散布，未確認的授權視同不允許，所以不能發佈"}
	}
}

func availabilityOf(status Status, skill SkillFacts, skillFound bool) Availability {
	switch {
	case status == StatusDelisted:
		return AvailabilityDelisted
	case !skillFound:
		return AvailabilityWithdrawn
	case skill.TakenDown:
		return AvailabilityTakenDown
	case skill.AccessRestricted:
		return AvailabilityHeld
	case releaseGate(skill, true) != nil:
		return AvailabilityNotRedistributed
	}
	return AvailabilityAvailable
}
