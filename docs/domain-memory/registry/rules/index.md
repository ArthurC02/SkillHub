# Business Rules

- [agent-daily-spend-cap-is-operator-adjustable-under-a-ceiling](/rules/agent-daily-spend-cap-is-operator-adjustable-under-a-ceiling) — A platform agent's effective daily spend cap is the operator's override when one is set, otherwise the default its definition declares.
- [batch-waits-only-on-a-passing-refusal](/rules/batch-waits-only-on-a-passing-refusal) — An evaluation batch waits when Run refuses a trial with a refusal Run says will pass on its own, and stops with that reason recorded on any other refusal.
- [creation-reaches-its-collaborators-through-inversion](/rules/creation-reaches-its-collaborators-through-inversion) — Interactive Skill creation does not import the Contexts it collaborates with, and none of them imports it.
- [dispatch-incident-halt-is-never-downgraded](/rules/dispatch-incident-halt-is-never-downgraded) — A dispatch halt declared as a P1 incident outranks a capacity pause on the same target: a later capacity pause replaces neither its source, its reason nor its….
- [run-terminal-status-is-final](/rules/run-terminal-status-is-final) — A Run that has reached a terminal status never transitions again.
