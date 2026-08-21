// Package identity owns login, sessions and resolving the authenticated user
// (CORE-005, ADR-020; ADR-032 §1 Identity & Workspace, requirement prefixes WS
// and SEC). Authorization stays workspace-scoped in SQL (ADR-011); this package
// only answers "who is calling" and "which workspace is theirs".
//
// # What it owns
//
// Four tables: `users`, `user_identities`, `sessions` and `workspaces`
// (db/query-owners.yaml: auth.sql, users.sql, workspaces.sql, and the
// identity-owned queries of governance.sql). It also owns the two deployment
// rosters that are configuration rather than data — the SEC-011 operator
// allowlist and the BETA-001 invite list — because both are read at the same
// moment as the session and neither has a grant endpoint to be owned by.
//
// # Relationships (ADR-032 §2)
//
// This context is Supplier to everyone and Customer to no one. Every other
// context imports it for SessionUser and PersonalWorkspace (iron rule 3's entry
// point), which means it may import none of them: `identity -> anything` is a
// compile-time cycle waiting to happen, and depguard would not need to say so.
//
// The account purge is the one place that constraint bites, and ADR-034 resolves
// it by inverting the direction. Five contexts own rows in a workspace being
// deleted — analytics, testlab, run, registry, ingest — and each decides for
// itself what deletion means for them (analytics de-identifies, registry keeps
// what a third party forked). Their steps arrive as injected [WorkspacePurge]
// values from the composition root, run inside this package's transaction, and
// are ordered by purge.go: registry before ingest, because ingest's step removes
// only the sources no surviving version points at.
//
// No domain events are published or consumed. A session is not something other
// contexts react to; they ask.
//
// # Public face
//
// Three things, and nothing else should be reached for:
//
//   - [SessionUser] — the authenticated user out of the request context, put
//     there by the middleware. The only sanctioned way to learn who is calling.
//   - Service.PersonalWorkspace — the scope every workspace-scoped read and
//     write is filtered by. A `workspace_id` that came from the request body is
//     iron rule 3's exact prohibition.
//   - Handler.RequireSession, RequireOperator, RequireInvited and
//     OptionalSession — the route-level gates. They stay per-route in
//     apiserver's router (AGENTS.md rule 10); a generated server mounted whole
//     would route around them.
//
// Service.LoginOrSignup, Logout, the account-deletion pair and the two sweeps
// (CleanupExpiredSessions, PurgeExpiredAccounts) are for the composition roots
// of cmd/api and cmd/maintenance.
//
// # What is deliberately not here
//
// Not authorization. This package answers identity; whether a caller may see a
// row is decided by that row's workspace scope in SQL, in the context that owns
// it (ADR-011). An operator check is the one exception and it is a route gate,
// not a rule about data.
//
// Not what any other context's rows mean when an account is deleted. That
// knowledge is theirs, which is the whole reason the purge steps are injected
// functions rather than statements written here.
//
// Not the analytics session. `sh_analytics` is a separate cookie in a separate
// package that must never be resolvable to a `sessions` row (ADR-029 決策 4).
//
// # Failure modes
//
// requirePurgeSteps refuses the entire batch before the worklist is even read
// when any context's step is missing. This is a compliance property, not
// tidiness: every other fail-closed check here guards against a wrong answer,
// this one guards against a purge that commits, reports success and leaves
// another context's rows in place — a user told their data is gone while it is
// still there, with nothing going red. Refusing is the recoverable outcome; the
// request stays on the worklist for a correctly wired deployment to finish.
//
// The purge transaction runs `SET LOCAL skillhub.purge = 'on'`, the one named
// exemption from the immutability triggers (0013, db/query-owners.yaml
// immutable_allow). It is scoped to that transaction and application code sets
// it nowhere else — which is also why the peers' steps must run inside this
// transaction rather than opening their own.
//
// Objects are removed before the transaction opens. Object storage has no
// rollback, so the two orders fail differently: this one can leave rows pointing
// at missing files in an account that is being deleted anyway and the next sweep
// finishes the job, while the other can leave a user's uploaded file alive with
// nothing in the database naming it.
package identity
