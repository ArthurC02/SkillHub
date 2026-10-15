// Package halt keeps the dispatch halt ledger and its detectors: which target
// dispatch stopped for and why, when it lifts, and the conditions that stop it
// on their own. An incident is never downgraded and never lifts itself.
package halt
