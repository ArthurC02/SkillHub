// Package halt keeps the dispatch halt ledger: which target dispatch stopped
// for, who stopped it and why, and when it lifts. An incident is never
// downgraded by a capacity pause and never lifts itself. Its Detector declares
// halts on its own: leaked sandboxes past a threshold, a masker that stopped
// redacting, a P-02 probe breach, and a stalled orphan reconciler.
package halt
