# Domain Registry schema

The Registry is a file-backed, reviewable projection of domain knowledge. It is not a substitute for the authoritative policy, contract, or decision source. Every record should contain a stable `id`, cited `evidence`, and fields appropriate to its type. An asset-level `status` is the default for its records; a record can override it. Use `candidate` until the named review process or owner confirms it; use `reviewed` only after that confirmation.

| Asset | Required domain meaning | Key fields beyond `id` and `status` |
| --- | --- | --- |
| Context | A business boundary and its responsibility | `name`, `subdomain`, `responsibility`, `business_owner`, `version`, `effective_from`, `implementation_path`, `requirement_prefixes` |
| Vocabulary | One Context-specific meaning | `name`, `definition`, `contexts`, `synonyms`, `not_same_as`, `required_qualifiers`, `owner` |
| Aggregate | An invariant boundary | `context`, `root`, `entities`, `value_objects`, `domain_services`, `policies`, `invariants`, `commands`, `allowed_dependencies`, `prohibited_dependencies` |
| Rule | A falsifiable business constraint | `contexts`, `statement`, `owner`, `version`, `effective_from`, `effective_until`, `examples` |
| Contract | An API or event promise | `kind`, `producer_context`, `consumer_contexts`, `version`, `compatibility_policy`, `data_classification` |
| Interaction | A collaboration across boundaries | `producer_context`, `consumer_context`, `consistency`, `delivery`; `contract_id` only when the collaboration relies on a stable API or event promise |
| Decision | A current architectural or business decision | `status` for the decision, `review_status` for Registry curation, `statement`, `source`, `effective_from`, `supersedes` |
| Event | A committed business occurrence | `owner_context`, `meaning`, `schema`, `consumers`, `version` |
| Capability | A business ability owned by one Context | `context`, `meaning` |
| Value object | An immutable domain concept | `context`, `meaning`, `fields` |
| Dependency policy | An allowed or prohibited Context collaboration | `from_context`, `to_context`, `mode`, `policy` |

A Context may record `confirmed_absences`: a list of `{asset, reason, evidence}` saying that the domain was examined and genuinely holds no vocabulary, aggregate, rule or contract for it. `coverage` then reports that Context under `confirmed_absent` instead of `gaps`, so an answered question stops looking like an open one. Declare an absence only when a source supports it; an unexamined Context belongs in `gaps`, and the validator refuses an absence that records contradict or that carries no reason.

A Context's abilities live in the Capability asset, one record each, and not in a field on the Context: one fact with two homes drifts, and only the asset can carry its own evidence and status.

A Context may say where it lives and what it owns in the surrounding process. `subdomain` is one of `core`, `supporting`, `generic` or `shared-kernel`, the strategic classification that decides how much the boundary is worth defending. `implementation_path` is the directory its code occupies, and two Contexts may not claim the same path or a path inside another's, because code belongs to one Context; sibling directories that merely share a name prefix do not overlap. `requirement_prefixes` lists the requirement-ID prefixes the Context owns. All three are optional, and a Registry that does not track code layout simply omits them.

`upsert-candidate --record-file` reads one record object, not a list and not a Change Package, and writes one record per call:

```json
{
  "id": "orders",
  "name": "Orders",
  "responsibility": "Own the order lifecycle.",
  "evidence": [{ "path": "docs/adr/boundaries.md", "lines": { "start": 12, "end": 18 },
                 "content_sha256": "sha256:...", "excerpt_sha256": "sha256:..." }]
}
```

Build each entry in `evidence` with `cite` rather than by hand.

`upsert-candidate` writes every record as a candidate whatever the submitted file says, because the command exists for machine-written records and a machine does not decide that a record has been reviewed. A record becomes reviewed only through an approved Change Package applied by `apply-approved-updates`.

The supplied validator checks file shape, asset formats, status values, IDs, Context references, contract references, and optional structured evidence. Reviewed records must contain their type-specific fields, evidence, and `review` metadata. `apply-approved-updates` writes that metadata from the approved Change Package. Use `validate --require-reviewed` before a change may rely only on reviewed records. It cannot confirm that a definition, owner, rule, or approval is true; review those claims against their authority.

For a time-bound rule or contract, record an ISO-8601 effective range. For an unknown owner or date, record `unknown`; do not omit the field or substitute a technical implementer.
