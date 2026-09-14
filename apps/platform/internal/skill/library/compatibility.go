package registry

type Capability string

const (
	CapabilityActivated    Capability = "activated"
	CapabilityNotActivated Capability = "not_activated"
	CapabilityUnverified   Capability = "unverified"
)

func AllCapabilities() []Capability {
	return []Capability{CapabilityActivated, CapabilityNotActivated, CapabilityUnverified}
}
