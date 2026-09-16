# Domain Registry maintenance

When a cited source changes, first determine whether it changes executable behavior, an intended rule, or only supporting prose. Produce a proposal that names the affected registry entries, source versions, and unresolved questions. Do not overwrite reviewed entries from a scan.

Use this sequence:

1. Run `probe --repo-root <repo> --registry-root <root>`; it names the sources that moved, and each selected path reports only the files it owns, so a change inside a nested source does not implicate the directory above it. Use `verify-sources` when the full snapshot detail is needed.
2. Identify the Context, vocabulary, rule, aggregate, interaction, or decision entry that may be affected.
3. Compare the source change with the existing definition.
4. Write a proposed update with its reviewer and effective status left unresolved when unknown.
5. Apply only the approved proposal in a normal reviewed change.

Evidence confirms where a statement came from. It does not establish that the statement is still the right business rule. Conflicting sources, obsolete decisions, and missing owners remain explicit gaps.
