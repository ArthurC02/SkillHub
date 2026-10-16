---
type: Business Rule
status: draft
owner: skill-packaging
version: "1"
effective_from: unknown
sources:
  - resource: "apps/platform/internal/skill/delivery/export.go#L28-37"
    excerpt_sha256: "sha256:144ac89c6b055019a6757a4461b7305f2c8c0ad656dc2ce6d87cf63a63ef3d0f"
  - resource: "apps/platform/internal/skill/delivery/export.go#L76-109"
    excerpt_sha256: "sha256:3675b017b98d8b9bbc2a8e93a9cde1fde82156532962433fd8de84acd5835901"
---
A package never carries version-control or tool-cache directories, credential files, non-regular files or paths that escape the package root; each is listed as an excluded file with its reason.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- .git/, node_modules/, .ssh/ and .kube/ directories are dropped (excluded_dir).
- .env and .env.production are dropped (credential_file), but .env.example, .env.sample and .env.template travel.
- A symlink or a path with a backslash or drive letter is dropped (not_a_regular_file, unsafe_path).
