# Extraction

Extraction turns a source into Registry candidates. It is judged by one question per element: does the source show this element as DDD defines it? A technique only proposes candidates; the definition decides.

## What each element is

| Element | It exists when the source shows | It is not |
| --- | --- | --- |
| Bounded Context | a boundary inside which one model and its terms mean one thing, owned and changed together | a folder, a service, or a team name on its own |
| Term | a word the domain uses with one meaning inside a Context, tied to behavior | an identifier, a table, or an acronym |
| Rule | a falsifiable statement the system enforces or refuses to break | a test name, a log message, or a wish |
| Capability | a business ability one Context owns and others ask of it | a handler, a package, or a feature flag |
| Aggregate | one invariant that must hold across several objects in a single consistency boundary, guarded by one root | an entity, a table, or a class with children |
| Domain Event | a business occurrence the domain cares about, after it is committed | a technical notification or a state write |
| Value Object | an immutable concept compared by its values | any data structure without an ID |
| Contract | a promise between processes or Contexts: shape, version and compatibility | an endpoint or a topic name |
| Interaction | one Context reacting to or calling another, with its consistency and, when the code shows it, the Context Map relationship between them | an import edge |
| Dependency policy | a direction one Context's code may or must never depend on | an observed import |
| Decision | a current, owned choice that constrains the model | a discussion or a superseded choice |

When the source shows something close to an element but not its defining property, record nothing, or record the weaker element that the source does show.

## The order of work

Models find vocabulary and relations reliably and Bounded Contexts and Aggregates unreliably, and an early wrong boundary spreads into everything recorded after it. So extract in this order, and validate after each stage before building on it:

1. **Language.** Collect the terms the domain uses and what the code or requirement does with each. A word with two meanings is a boundary signal, not one term. Keep them as notes: a Term names its Contexts, so it is recorded once step 2 has them.
2. **Boundaries.** Propose Contexts only where several signals agree: the language changes, data is owned and written in one place, changes land together, and calls cross the boundary through an explicit surface. Each Context names its `implementation_path` when code exists. Prefer fewer, evidenced Contexts to a speculative split.
3. **Relations.** Record Interactions and Contracts across the boundaries from step 2. When the work covers one Context and a relation needs a neighbour, record the neighbour as a candidate Context holding only what the evidence shows, such as its `id`, `name` and `implementation_path`, so the relation can name it; Review decides whether it holds. Record a Dependency policy only for a direction a check enforces or a decision states.
4. **Tactical facts.** Rules, Aggregates, Events and Value Objects, each inside one Context from step 2. A Rule that states an Aggregate's invariant and an Event that Aggregate raises name it in `aggregate`; an Event published under a Contract names it in `contract_id`.

## From a requirement

Read the intended behavior and its acceptance criteria. Resolve each noun with `resolve-terms` before inventing a term. Name the Context that would own the decision, the rules the criteria make falsifiable, the events the criteria observe, and the collaborations the behavior needs. Anything the requirement leaves open is an open question in the Change Package, not a guessed record; continue with the Design capability, `domain-memory-design`, for a material change.

## From code

Treat the code as the evidence of current behavior, not of intent. Read the boundary surfaces first: public APIs, event schemas, persistence ownership, composition roots and architecture checks. Then read inside each candidate Context for its terms, guards and refusals. Tests that assert a refusal are good evidence for a rule; a test name alone is not.

`change-coupling --repo-root <repo> --root <code root> --depth <n>` reports which modules change together in history.

- Strong coupling inside a candidate Context supports it; strong coupling across two candidates asks whether they are one Context or share a hidden contract; a composition root couples with everything and says nothing about boundaries.
- It reports only modules that still exist; use `--since` to keep a rewritten history from dominating.
- Files outside every root, such as a web client, contracts or migrations shipped with the same feature, do not count toward `--max-changeset`, since a sweeping rename or format couples everything.
- `--max-modules` also skips a commit that touches more modules than that, which catches a sweep of few files spread wide. The newest twenty are listed in `skipped_sweeping` with their `modules` and, with `--registry-root`, their `contexts`, so a reader can check each is a sweep.
- With `--registry-root` only modules a Context owns count toward `--max-modules`, since nearly every feature also touches code no Context owns, such as wiring or generated types.
- A feature that fans in, one Context consuming several others in one commit, also crosses the limit: read the `contexts` of each `skipped_sweeping` entry, and run once without `--max-modules` before concluding that a hub Context has no visible collaborations.
- A commit whose subject matches an `--exclude-subject` pattern, such as `^(chore|style|test)` (`test(` subjects are often sweeps too), is left out, because a repository-wide sweep couples modules that share no domain.
- The `subjects` of each pair let a reader tell a domain change from a sweep.
- When a module lies in no Context, `explained_by` is `null`, since the Registry cannot speak for it.

`compare-registry` scores an extraction against a gold Registry, and pairs records this way:

- A Context pairs by its `implementation_path`: the same path (`path`), one ending with the other, as when one Registry writes it from the repository root and the other from a module (`path-suffix`), or one inside the other (`nested-path`); otherwise by name.
- Interactions and dependency policies pair when both of their Contexts pair.
- Any other record pairs by id or name, ignoring case and punctuation (a term also by a gold synonym, an aggregate by its root), or by naming a paired Context and citing a file the gold record cites. Among those, the pair whose cited lines overlap most is taken first, so a record citing the same lines wins over one citing elsewhere in the file.

Reading the score:

- `--context` is for an extraction that covered part of the code: candidate records count only when they name a candidate Context paired with a scoped one, so an extraction of other code does not lower the precision. Contexts outside the scope still pair, so an interaction or dependency policy between a scoped Context and another one is scored.
- Each `overall_*` score carries its `matched`, `gold` and `candidate` counts, so a ratio over two records reads as such.
- The evidence of a pair other than a Context pair is usually the package documentation either side would cite, so it is marked `copied_evidence` when one side's cited files and excerpts are all among the other's, as when a harvest dropped a citation, since a blind extraction rarely cites exactly the gold lines.
- It measures structure, not meaning: a rule worded differently but citing other lines counts as missed. Read `possible_pairs` and the missed and extra lists before trusting a number.

## From a change

Start from what moved: `boundary-impact --base <ref>` for the Contexts and evidence a change touched, `change-coupling --since <date>` for how the change pattern shifted, and the diff or the changed requirement itself. Then decide, element by element, whether the model changed: a term, the owner of a decision, an invariant or rule, a boundary or allowed dependency, an event or contract, or a capability. A change to who may decide a value, its bounds, or when it is decided is a rule change; moving a value from deployment configuration to a setting someone adjusts at run time is one. A refactoring that moves code and keeps all of these is not a model change; re-cite its evidence with `relocate-evidence` and record nothing new.

## What a record holds

Only the element, its relations and its evidence. Never the technique that found it, the steps taken, a confidence score, a session note or a to-do. Unresolved points belong in the Change Package's open questions. A shorter record that a reviewer can check against its citations is better than a complete-looking one that restates the source.
