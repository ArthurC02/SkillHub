---
type: Business Rule
status: draft
---
A source is read as an Agent Plugin only when the plugin.json in its root declares the specification's canonical schema identifier and then conforms to it: a name of 1 to 64 lowercase alphanumerics, hyphens and periods whose first and last characters are alphanumeric and which holds no consecutive hyphen or period, and an author, where present, that is an object carrying only name, email and url, each a string. A manifest that declares another specification, declares none, or is not valid JSON is disclosed and the source is read as a directory tree instead; a manifest that declares this specification and breaks it refuses the whole source, names the field, and leaves its components undiscovered.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A manifest whose author carries twitter is refused, the refusal names twitter, and none of the skills under skills/ are discovered.
- A manifest declaring another vendor's schema identifier is disclosed at info level and its skills are imported as a directory tree.
- An extensions field that is not an object is disclosed and ignored, and the plugin still loads.
