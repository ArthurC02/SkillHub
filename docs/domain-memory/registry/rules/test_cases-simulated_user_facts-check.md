---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0121_simulated_user.sql#L1-2"
    excerpt_sha256: "sha256:be663b99e36b857999231642ab5c5826a0d86f4f483410858b33c3dd88fe1a97"
---
test_cases keeps check test_cases.simulated_user_facts: simulated_user_facts text[] NOT NULL DEFAULT '{}' CONSTRAINT test_cases_simulated_user_facts_count CHECK (cardinality(simulated_user_facts) <= 10)

# Relations
- Contexts: [testlab](/contexts/testlab)
