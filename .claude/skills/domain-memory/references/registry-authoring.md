# Domain Registry authoring

Create a registry only after identifying the repository's applicable instructions and source hierarchy. Initialize the supplied layout, then curate one bounded slice at a time.

The registry has eleven asset types:

| Asset | Records | Must not be inferred from |
| --- | --- | --- |
| Context | boundary, responsibility, owner status | a folder name alone |
| Vocabulary | name, definition, Context, evidence | identifiers or uppercase words |
| Aggregate | invariant boundary, commands, evidence | a database table or service class |
| Rule | falsifiable statement, scope, evidence, owner status | a test name alone |
| Contract | API or event promise, producers, consumers, and compatibility | an endpoint or topic name alone |
| Interaction | producer, consumer, consistency, evidence | an import edge alone |
| Decision | current status, statement, source | an ADR filename without status |
| Event | committed business occurrence, owner, schema | an event topic or database mutation alone |
| Capability | business ability owned by a Context | a UI route or service name alone |
| Value object | immutable concept and its fields | a JSON payload shape alone |
| Dependency policy | allowed or prohibited Context collaboration | an import edge alone |

Each entry is `candidate`, `reviewed`, `deprecated`, or `superseded`. A reviewed entry must name an authorized review process or owner. Record version and effective period when the domain asset changes over time. When no owner or date is known, record `unknown`; do not substitute a technical package owner.

Start with the smallest slice that contains a real cross-context behavior: two Contexts, a shared vocabulary term, an invariant, and one interaction. Use the templates as a starting point, not as a claim that every repository needs every asset type.
