// Package envx reads process environment with defaults. Generic: no domain
// rules. Fail-closed checks on required secrets stay at their call sites.
package envx

import "os"

// Or returns the value of key, or fallback when it is unset or empty.
func Or(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
