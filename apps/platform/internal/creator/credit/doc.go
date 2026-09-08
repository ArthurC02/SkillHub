// Package credit owns the platform's only unit of account (ADR-068): a
// user's credit balance, the two ledgers that explain it, and the
// statistics that turn "how much things cost" into "should this be allowed
// to start". cost_events is the platform's real USD spend, per paid call;
// credit_entries is what a user's account was actually charged after
// markup — they are never merged into one table (ADR-068 decision 3), and
// the materialized balance (credit_accounts.balance_credits) is the sum of
// a user's entries, updated in the same transaction that writes them
// (decision 4). Migration 0060 has already created all four tables
// (cost_events, credit_accounts, credit_entries, cost_statistics) and
// db/queries/{credit,cost}.sql already name every query this package's
// Store port needs — see store.go's per-method comments for the mapping.
// `task gen:sql` has not been run for them yet (main-agent-serialized,
// ADR-032 §1's own note on this); this package's [Store] interface is what
// lets everything else here compile and be tested without waiting on that.
//
// Accounts are keyed by user, not workspace (migration 0060's own comment:
// ADR-068's "每個帳號" is the user — MVP's 1:1 user/workspace does not change
// that). A workspace-scoped caller resolves to the owning user id before
// calling in; see this batch's report for exactly where that resolution
// belongs.
//
// creation, ingest and eval are this package's customers for [Service.
// Charge] (ADR-032 §1): they call in before spending (CanStart,
// CanAffordStep) and after (Charge). catalog is a customer for
// [Service.RecordCost] only — its search-embedding and index-enrichment
// calls cost real money but are not (yet, MVP) billed to any account. credit
// calls back out to identity for one fact only — whether a user still
// exists and is not purged — through the injected [Service.Facts] func, the
// same convention skill/discovery's SkillFacts and skill/admission's
// VersionFacts already use; credit itself never imports identity.
//
// Every rule ADR-068 states — the ceiling conversion (money.go), the
// idempotent charge, the -50 floor, the p95 threshold with its
// sample-count fallback (service.go) — is implemented and tested here
// against a fake Store, so the Postgres adapter has nothing left to decide.
package credit
