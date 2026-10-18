---
name: domain-memory-review-change
description: Review a pull request or a branch for what it does to the reviewed domain model, starting from one brief, and decide whether it needs a Change Package, a recite, or nothing.
---

# Review a change against Domain Memory

Use this when a branch is ready for review and the Registry holds reviewed records. It reads the change through the model; the code review itself is still yours.

1. Run `brief --registry-root <root> --repo-root <repo> --base <branch point> --markdown` and paste it into the review. The branch point is the commit the branch grew from, not the branch tip. The JSON form, without `--markdown`, is the same brief for a tool.
2. Read the verdict first:
   - `unregistered-co-change`: two Contexts changed together and no interaction, contract or dependency policy joins them. Ask for the collaboration to be registered through a Change Package, or for the change to be split so each Context changes alone.
   - `boundary-evidence-changed`: a collaboration's cited lines changed. For each record under *Records to re-read*, compare the statement with the diff. If the code still states the fact, ask for `recite`; if it no longer does, the change is material and needs a Change Package, or a candidate with `upsert-candidate` when the author only noticed the drift.
   - `registered-co-change`: the Contexts changed together through registered collaborations. Check that each one listed still holds; a changed message shape or direction is a contract change.
   - `no-cross-context-change`: the change stays inside one Context. Read that Context's rules in the brief and check the diff against them.
   - `unknown`: no Context records an `implementation_path`, so nothing could be mapped. Ask for the paths before relying on the rest of the brief; see [Registry authoring](../../references/registry-authoring.md).
3. *Records to re-read* carries each record's dependents: the records that reach it through relations. A change to the record reaches them too, so name them in the review when the record changes.
4. *Evidence* counts the citations of the changed files. `stale`, `missing`, `unhashed` or `invalid` citations are review gaps: ask for `recite`, and do not treat the records that carry them as verified until it ran. See [evidence rules](../../references/evidence-rules.md).
5. *Attention* is the review's checklist: every line either becomes a request to the author or is answered in the review. A brief with one line of attention and `no-cross-context-change` is a change the model has nothing against.

Reviewing is read-only. Do not promote candidates, approve proposals or recite on the author's behalf; a Change Package, when one is needed, follows [the seven-step workflow](../../references/seven-step-workflow.md). Command shapes are in [the file-backed API](../../references/script-api.md).
