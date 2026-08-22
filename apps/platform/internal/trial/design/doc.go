// Package testlab is the current Go package for ADR-038's proposed product
// domain 「試跑情境設計」: it lets creators define prompts, data, acceptance
// criteria and permission confirmation for a trial. Its stable Boundary
// ID/package slug remains `testlab`; ADR-038 is Proposed and does not rename it.
// It owns test cases, their acceptance criteria, their uploaded datasets, and the immutable snapshot a run executes (ADR-032 §1 Test Lab, requirement prefix TEST; TEST-001/003/004/010, ADR-002).
//
// The split that matters here: a test case is an editable draft, a
// test_case_snapshots row is the frozen fact of what one run actually used
// (ADR-003, iron rule 4). Editing a draft never rewrites what a past run did.
//
// Nothing in this package executes uploaded content. Datasets are stored as
// opaque bytes and only ever unpacked inside the sandbox (iron rule 1).
//
// # What it owns
//
// Three tables — `test_cases`, `datasets`, `test_case_snapshots`
// (db/query-owners.yaml: test_lab.sql, plus ListWorkspaceObjectKeys,
// DeleteWorkspaceDatasets, ListDatasetsClaimingObject, MarkDatasetObjectLost and
// ListTestCasesForSkill, which sit in other files because of who scans them, not
// because of who owns them). `test_case_snapshots` is declared immutable: a
// snapshot that can be edited makes a past run's inputs lie.
//
// # What other bounded contexts may use (ADR-032 附錄 A)
//
// The package is not split into sub-packages; its public surface is, and this is
// the whole of it:
//
//   - Write: [CreateSnapshot], the only place a test case is frozen for a run.
//     Called inside the run creation transaction.
//   - Read: [ReadDraft], the editable test case as another context sees it.
//     Reading test_cases / datasets through gen directly is the thing this
//     replaced — internal/run had grown its own dataset type and its own copy of
//     the ordering comment, which is two definitions of one fact.
//   - Lock: [LockDraft], the same read with the row lock, for a caller whose
//     critical section has to open before its own checks. DDD-031 added it so
//     that internal/run could stop taking this package's lock with this
//     package's query: a lock and the invariants it protects belong to one
//     context, or the owner cannot say whether its writes were serialised
//     (ADR-035 B 組).
//   - Read (other contexts' entry points, all taking the caller's *gen.Queries):
//     [ReadSnapshot] for the frozen inputs a run executed, [ReadDataset] for one
//     live file's current object key, [CasesForSkill] and [CaseDatasets] for a
//     Skill's cases and their files. DDD-033 added them so eval, run and
//     packaging could stop calling this package's queries: nothing was at risk,
//     but a query called from outside freezes this schema against callers this
//     package has no way to find (ADR-035 C 組).
//   - Decode: [DecodeCriteria], [DecodeRubric], [DecodeDatasetRefs]. These are the
//     "read it the way it was written" guarantee; a caller that reaches for
//     encoding/json on one of these columns has opted out of it.
//   - Types: [Criterion], [Rubric], [RubricItem], [DatasetRef], [DatasetFile], and
//     the sentinel errors in testlab.go.
//   - Limits: the constants in testlab.go are the only enforcement point. GET
//     /test-cases/limits is their display projection, not a second copy.
//   - Injected: [PurgeWorkspace] and [WorkspaceObjectKeys] for identity's account
//     purge, [MarkDatasetObjectLost] for objreconcile's sweep. All three take the
//     caller's *gen.Queries and open nothing of their own. The two purge halves
//     are separate because they run at different moments — keys before the
//     transaction, rows inside it, and objects have no rollback.
//
// # Relationships (ADR-032 §2)
//
// All four inbound edges are synchronous queries (Customer–Supplier); nothing
// here is driven by or publishes a domain event.
//
//	run -> testlab          LockDraft then CreateSnapshot inside the create-run
//	                        transaction, ReadDraft at preflight, ReadSnapshot and
//	                        DecodeDatasetRefs when scheduling, and ReadDataset when
//	                        issuing object grants.
//	eval -> testlab         ReadSnapshot, then the criteria and rubric a verdict is
//	                        measured against through the Decode* functions.
//	packaging -> testlab    CasesForSkill and CaseDatasets: the Test Cases that may
//	                        travel in a download, and the files that decide whether
//	                        they may.
//	testlab -> llmclient    ACL. TEST-002 criteria suggestions; the model returns
//	                        text and nothing else (iron rule 6).
//	testlab -> identity     synchronous, iron rule 3.
//	identity, objreconcile  reach this package through injected functions rather
//	                        than imports (ADR-034, ADR-033 clearance path 4).
//
// # What is deliberately not here
//
// Not what a run does with a snapshot. Once CreateSnapshot returns, the input is
// run's record of what it was given, and this package never touches it again —
// which is also why the account purge deletes datasets but leaves snapshots
// standing.
//
// Not the judgement. This context owns what an acceptance criterion *is*; whether
// a run met it is eval's verdict, written to `evaluations` (ADR-025). A criterion
// edited after a run does not change what that run was judged against, because the
// judgement was made against the snapshot.
//
// Not the enforcement of snapshot immutability, which is the 0005 trigger plus
// db/query-owners.yaml's `immutable:` declaration — this package's discipline is
// not the mechanism.
//
// Not unpacking. filetype.go judges a dataset by magic bytes and refuses archives
// it will not open; opening one is the sandbox's job, never this process's.
//
// # Failure modes
//
// Every PDM-005 §5.1 limit is enforced in UploadDataset and only there: 25 MB per
// file, 100 MB and 20 files per test case (both under a lock, against the live
// total), allowed types by magic bytes with the file name never consulted, and a
// 90 day expiry written at creation. A second copy of any of those numbers is how
// the endpoint and the limits page start disagreeing.
//
// The dataset object lands in storage before the row commits, so a rolled back
// transaction leaves an orphan object rather than a row pointing at nothing, and
// the key is per-dataset rather than content addressed so deleting one dataset can
// never remove bytes another still references. The sweep that notices the opposite
// drift — a row claiming a file that is gone — marks through
// MarkDatasetObjectLost.
//
// The rejection message is identical for every unsupported type on purpose
// (02:TEST-002): the caller never learns which magic bytes were detected or which
// rule fired.
//
// A missing criteria suggester is never fatal — automatic suggestion is 可選強化
// in 02:TEST-001, so the manual path is the fallback and ErrSuggestUnavailable
// says so.
package testlab
