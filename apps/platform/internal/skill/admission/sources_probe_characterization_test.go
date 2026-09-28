package ingest

func probeOf(available, changed bool) sourceProbe {
	switch {
	case !available:
		return sourceUnavailable
	case changed:
		return sourceContentChanged
	default:
		return sourceUnchanged
	}
}
