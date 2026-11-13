package sandbox

import "context"

type WebSearchRuntimeReader interface {
	WebSearchRuntime(context.Context) (RuntimeProfile, error)
}

func (m *Manager) runtimeCapabilities(ctx context.Context) []RuntimeCapability {
	var actual RuntimeProfile
	var err error
	if reader, ok := m.drv.(WebSearchRuntimeReader); ok {
		actual, err = reader.WebSearchRuntime(ctx)
	}
	var result []RuntimeCapability
	for _, configured := range m.cfg.Runtimes {
		for i := len(configured.Versions) - 1; i >= 0; i-- {
			rt := configured
			rt.Versions = []string{configured.Versions[i]}
			rt.WebSearch = ""
			switch {
			case err != nil:
				rt.WebSearch = "unavailable"
			case actual.Runtime != "" && rt.Runtime == actual.Runtime && rt.Versions[0] == actual.RuntimeVersion:
				rt.WebSearch = "supported"
			}
			result = append(result, rt)
		}
	}
	return result
}
