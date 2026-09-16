# Proposal lifecycle

Use a Domain Change Proposal for a change that adds or changes an Aggregate, rule, event, public contract, Context boundary, or consistency model. The proposal is a design record; it does not grant permission to modify a repository.

| Status | Meaning | Permitted transition |
| --- | --- | --- |
| `draft` | Evidence and design are still being assembled. | Run `submit-proposal` only after affected Contexts, risks, and open questions are explicit. |
| `submitted` | Awaiting evidence collection and required review against a Registry revision. | Record each human approval, then run `verify-proposal`. |
| `verified` | Every planned obligation has a supplied passing, digest-backed test attestation. | Add SCM proof and run `finalize-proposal`. |
| `approved` | A named authority accepted this verified proposal against its recorded base revision. | Apply within its approved scope. |
| `rejected` | The proposal must not be implemented. | Create a new draft; do not overwrite the rejected record. |
| `superseded` | Evidence or a decision overtook this proposal before it was applied, or a later proposal replaced it. | Run `supersede-proposal` with a reason, and `--superseded-by` once the replacement has an id. Create the replacement as a new draft. |
| `applied` | The approved scope was implemented and linked to evidence. | Do not alter the approved content; supersede it with a new proposal. |

A proposal in any status other than `rejected` or `superseded` may be superseded. Withdrawing it silently, or editing a submitted proposal back into a draft, destroys the one thing the package exists to carry: why a conclusion was reached and why it stopped holding. The superseded record keeps the status it died in, the reason, and the replacement.

Record each approval with the reviewer role, reviewer identity, timestamp, scope, proposal revision, and base Registry revision. The `record-approval` command rejects self-approval and duplicate approval of the same revision. The base revision contains an observed Git HEAD for provenance and a Registry digest for the concurrency check; it never searches Git history. Finalization and application reject a changed Registry digest. An unrelated Git commit does not invalidate the Proposal.

The Skill prepares proposals and identifies required review. It must not fabricate approval, treat a comment as approval, or bypass repository, CI, change-management, or deployment controls.
