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

A proposal in any status other than `rejected` or `superseded` may be superseded. Withdrawing it silently, or editing a submitted proposal back into a draft, destroys what the package exists to carry: why a conclusion was reached and why it stopped holding. The superseded record keeps the status it died in, the reason, and the replacement.

Record each approval with the reviewer role, reviewer identity, timestamp, scope, proposal revision, and base Registry revision. `record-approval` rejects self-approval and duplicate approval of the same revision. The base revision contains the observed Git HEAD and a Registry digest for the concurrency check. When the digest has changed, verification, finalization and application rebuild the base Registry from that commit, and accept the Proposal only if the rebuilt Registry reproduces the base digest and the policy, the manifest, every asset outside its records, the records the Proposal writes or removes, and the records those name are unchanged. Anything else is refused as stale and needs fresh approval, naming what changed. An unrelated Git commit or another Proposal's apply therefore does not invalidate it; a base captured from uncommitted Registry files cannot be rebuilt and goes stale on any change. Application installs against the digest it just compared, so a write in between is still refused, and its audit event records the revision it applied on top of, with the Proposal's base beside it when they differ.

The Skill prepares proposals and identifies required review. It must not fabricate approval, treat a comment as approval, or bypass repository, CI, change-management, or deployment controls.
