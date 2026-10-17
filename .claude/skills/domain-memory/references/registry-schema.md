# Domain Registry schema

The Registry is a file-backed, reviewable projection of domain knowledge, not a substitute for the authoritative policy, contract, or decision source. Every record should contain a stable `id`, cited `evidence`, and fields appropriate to its type. An asset-level `status` is the default for its records; a record can override it. Use `candidate` until the named review process or owner confirms it; use `reviewed` only after that confirmation.

| Asset | Required domain meaning | Key fields beyond `id` and `status` |
| --- | --- | --- |
| Context | A business boundary and its responsibility | `name`, `subdomain`, `responsibility`, `business_owner`, `version`, `effective_from`, `implementation_path`, `requirement_prefixes` |
| Vocabulary | One Context-specific meaning | `name`, `definition`, `contexts`, `synonyms`, `not_same_as`, `required_qualifiers`, `owner` |
| Aggregate | An invariant boundary | `context`, `root`, `entities`, `value_objects`, `domain_services`, `policies`, `invariants`, `commands`, `allowed_dependencies`, `prohibited_dependencies` |
| Rule | A falsifiable business constraint | `contexts`, `statement`, `aggregate` (the Aggregate whose invariant it states; must exist and sit in a Context the rule names), `owner`, `version`, `effective_from`, `effective_until`, `examples` |
| Contract | An API or event promise | `kind`, `producer_context`, `consumer_contexts`, `version`, `compatibility_policy`, `data_classification`, `idempotency` |
| Interaction | A collaboration across boundaries | `producer_context`, `consumer_context`, `relationship` (the Context Map pattern between them: `partnership`, `shared-kernel`, `customer-supplier`, `conformist`, `anticorruption-layer`, `open-host-service`, `published-language` or `separate-ways`; the producer is upstream), `consistency`, `delivery`; `contract_id` only when the collaboration relies on a stable API or event promise |
| Decision | A current architectural or business decision | `status` for the decision, `review_status` for Registry curation, `statement`, `source`, `effective_from`, `supersedes` |
| Event | A committed business occurrence | `owner_context`, `meaning`, `schema`, `aggregate` (the Aggregate that raises it; must exist and sit in `owner_context`), `contract_id` (the Contract it is published under; must exist), `consumers` (the Context ids that react to it; each must exist), `version` |
| Capability | A business ability owned by one Context | `context`, `meaning` |
| Value object | An immutable domain concept | `context`, `meaning`, `fields` |
| Dependency policy | An allowed or prohibited Context collaboration | `from_context`, `to_context`, `mode`, `policy` |

A Context may record `confirmed_absences`: a list of `{asset, reason, evidence}` saying the domain was examined and genuinely holds no vocabulary, aggregate, rule, contract or event for it. `coverage` then reports that Context under `confirmed_absent` instead of `gaps`, so an answered question stops looking open. Declare an absence only when a source supports it; an unexamined Context belongs in `gaps`, and the validator refuses an absence that records contradict or that carries no reason.

A Vocabulary term's `name` is its one preferred name. `synonyms` lists other names people use for the same meaning, which code and documents should avoid in favour of `name`; `resolve-terms` finds the term through them, and a lint can flag them. `not_same_as` lists the ids of other terms that look or sound alike but mean something different. Both are optional lists. The validator refuses a blank or repeated synonym, a term's own name as its synonym, a synonym that is another term's name in a Context they share, and a `not_same_as` entry that is not another term's id. A synonym may be another term's name in a different Context, because each term means one thing within its Contexts.

A Contract may state how a repeated request behaves in an optional `idempotency` object, because the producer that accepts the retry is the one that promises it. `key` names what identifies one request, `scope` says where that key must be unique, `on_replay` says what the same request with the same key gets, `on_conflict` says what a different request with the same key gets, and an optional `expiry` says when a key may be reused. Each is plain text. For example, `{"key": "origin key", "scope": "per Workspace", "on_replay": "returns the Run already started", "on_conflict": "refused when the Skill Version differs"}`. The validator refuses a missing, blank, or unknown field. `analyze-boundary` prints it with the Contract.

When one Context publishes a fact and another decides what to do about it, record each where its owner is: the fact in the producer's Contract, such as "this refusal is temporary", and the decision as a Rule in the consumer's Context, such as "wait for a temporary refusal, stop on any other". The Interaction joins them through `contract_id`. Do not copy the policy into the Contract or the fact into the Rule: one fact with two homes drifts.

An outside system, such as a provider service another process runs, becomes a Context only when the Registry must name a promise it makes: record it with `subdomain: generic` and a responsibility saying what it provides, and the Contract with it as `producer_context`. Otherwise record only what the consuming Context decides, as its Rules and Interactions. Never name the consumer as the producer of its own dependency.

When one Context declares a port that several others implement, each implementer provides the service: record one Interaction per implementer with the implementer as `producer_context` and the declaring Context as `consumer_context`.

A dependency policy's `policy` is `allowed` or `forbidden`, and its `mode` names how the code depends: `synchronous-query` for a read, `synchronous-write` for a call that changes the other Context, `asynchronous` for an event, and `none` for a direction that must not exist; `validate` refuses any other value of `policy`, `mode` or an Interaction's `relationship`.

A Context's abilities live in the Capability asset, one record each, not in a field on the Context: one fact with two homes drifts, and only the asset can carry its own evidence and status.

A Context may say where it lives and what it owns in the surrounding process. `subdomain` is one of `core`, `supporting` or `generic`, the strategic classification that decides how much the boundary is worth defending; a shared kernel is not a kind of Context but a relationship between two, recorded as `relationship: shared-kernel` on their Interaction. `implementation_path` is the directory its code occupies, such as `apps/platform/internal/trial/execution`; a file belongs to it when that directory sequence occurs anywhere among the file's directories, so `internal/trial/execution` owns the same files, and `validate --repo-root` refuses a reviewed Context's path under which no repository file lies and notes a candidate's; two Contexts may not claim the same path or a path inside another's, because code belongs to one Context, while sibling directories that merely share a name prefix do not overlap. `requirement_prefixes` lists the requirement-ID prefixes the Context owns. All three are optional; a Registry that does not track code layout omits them.

A field whose value the source does not show is left out, never filled with a placeholder or a guess; the validator asks for what a reviewed record needs.

`upsert-candidate --record-file` reads one record object per file, not a list and not a Change Package. Give `--record-file` once for each record of the same asset that belongs to one change, such as two terms that name each other in `not_same_as`: the records are validated together and written in one audited update, or none of them is written:

```json
{
  "id": "orders",
  "name": "Orders",
  "responsibility": "Own the order lifecycle.",
  "evidence": [{ "path": "docs/adr/boundaries.md", "lines": { "start": 12, "end": 18 },
                 "excerpt_sha256": "sha256:..." }]
}
```

Build each entry in `evidence` with `cite` rather than by hand.

`upsert-candidate` writes every record as a candidate whatever the submitted file says, because the command exists for machine-written records and a machine does not decide that a record has been reviewed. A record becomes reviewed only through an approved Change Package applied by `apply-approved-updates`.

## Concept files

The Registry stores each record as one Markdown concept document in the [Open Knowledge Format](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md): `registry/<asset>/<id>.md`, so a record's concept ID is `/<asset>/<id>`. `registry/index.md` declares the OKF version and lists the assets, and each asset folder's `index.md` lists its records with their first sentence; the scripts write both. Read the Registry through the commands, or `export`, rather than parsing these files.

```markdown
---
type: Interaction
status: stable
consistency: synchronous
sources:
  - resource: "src/registry/facts.py#L196-219"
    excerpt_sha256: "sha256:0e7d…"
---
# Relations
- Producer context: [registry](/contexts/registry)
- Consumer context: [run](/contexts/run)
- Contract id: [registry-run-facts-v1](/contracts/registry-run-facts-v1)
```

- `type` names the asset. A Context or term's `name` is the `title`.
- `status` is `draft` for a candidate, `stable` for a reviewed record and `deprecated` for a deprecated one. A superseded record is `deprecated` with `registry_status: superseded`. A Decision's own status is `decision_status`.
- `evidence` is `sources`: each citation's path and lines become one `resource`, `path#Lstart-end`. A citation keeps only what checking it needs, its `excerpt_sha256`; the kind of source it comes from is derived from `source-map.json` whenever it is verified.
- `verified` lists who approved a reviewed record and when, each entry `by: "human:<reviewer>"` and `at`. The proposal that reviewed it is in the audit log, not the record.
- The body opens with the record's prose field, such as a Context's responsibility, a term's definition or a rule's statement.
- Then `# Relations` lists each reference to another record as `- <Field>: [id](/<asset>/<id>)`, one line per target. This is how a reader follows the knowledge graph.
- Each list of short texts, such as a rule's `examples`, becomes its own `# <Field>` section of `-` items.
- Every other field stays in the frontmatter, as does any value Markdown cannot carry exactly: a multi-line item, an empty list, or a reference that is not an id. A record therefore reads back exactly as written.

The frontmatter uses a subset of YAML: `key: value` lines, nested by two spaces, `- ` items, and values either plain or in double quotes. A hand edit that goes beyond it, such as single quotes or `[a, b]`, is refused with its line number.

The supplied validator checks file shape, asset formats, status values, IDs, Context references, contract references, and optional structured evidence. Reviewed records must contain their type-specific fields, evidence, and `verified`, which `apply-approved-updates` writes from the approved Change Package. A record still holding an older `review` block is refused until `migrate-registry` refines it. Use `validate --require-reviewed` before a change may rely only on reviewed records. The validator cannot confirm that a definition, owner, rule, or approval is true; review those claims against their authority.

For a time-bound rule or contract, record an ISO-8601 effective range. For an unknown owner or date, record `unknown`; do not omit the field or substitute a technical implementer.
