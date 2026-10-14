// Package halt keeps the dispatch halt ledger: which target dispatch stopped
// for, who stopped it and why, and when it lifts. An incident is never
// downgraded by a capacity pause and never lifts itself.
package halt
