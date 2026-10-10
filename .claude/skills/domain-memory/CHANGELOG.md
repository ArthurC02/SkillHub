# Changelog

Notable changes to the Domain Memory plugin, newest first. The format follows Keep a Changelog, and versions follow Semantic Versioning: a minor version adds or changes behavior, a patch version only fixes it. Versions before 0.10.0 are recorded in the repository history only.

## 0.10.9

### Changed

- Citation verification is split into checking the reference's shape, its source file, and its cited lines; every verdict is unchanged.

## 0.10.8

### Changed

- Audit chain verification is split into checking each event, following the chain, and comparing the manifest; every verdict and reason is unchanged.

## 0.10.7

### Changed

- GitHub pull request verification is split into fetching its state and judging the merge, the current approval and the checks; every message is unchanged.

## 0.10.6

### Changed

- Policy validation is split into its enumerated fields, review governance, sources and limits; every message, and governance reporting only its first problem, is unchanged.

## 0.10.5

### Changed

- Approval validation is split into each approval entry and the fields each lifecycle stage requires; every message is unchanged.

## 0.10.4

### Changed

- Applying approved updates is split into finding the finalized proposal, parsing each update, and applying it; every refusal and its message is unchanged.

## 0.10.3

### Changed

- Source map verification is split into its structural checks and the snapshot comparison; every verdict and reason is unchanged.

## 0.10.2

### Changed

- Recovery of an interrupted Registry update is restructured into one handler per journal phase; what it does in each phase is unchanged.

## 0.10.1

### Fixed

- A file-system error, such as `init-change-package --output` naming a file, ends in one `ERROR:` line and exit code 1 instead of a Python traceback.
- `verify-audit` reports `invalid` for a `--registry-root` that holds no Registry, instead of calling an absent audit chain valid.

## 0.10.0

### Changed

- When another proposal's apply changes the Registry, `verify-proposal`, `finalize-proposal` and `apply-approved-updates` rebuild the base Registry from the proposal's observed commit and accept it if the records it touches are unchanged; otherwise it is still stale, and the refusal names what changed. Commit the Registry before `submit-proposal` to benefit.
- `record-test-result` runs its command without a shell, split by POSIX quoting rules on every platform. Pipes, `&&` and `VAR=value` prefixes are no longer available.

### Fixed

- `record-test-result` ran from the command line, and accepts any profile when the policy approves none.
