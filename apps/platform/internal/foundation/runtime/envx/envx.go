package envx

func Or(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
