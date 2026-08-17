#!/usr/bin/env python3
"""PACK-001/002/003/005 contract check for contracts/packaging/.

Every example in each of the three schemas validates, and one counterexample per
failure mode that actually matters is rejected. The counterexamples are the half
that carries the argument: a schema that accepts everything still passes its own
examples, so the boundaries this milestone cares about - a licence expression
without its provenance tier, a compatibility verdict without the image it was
measured on, evaluation prose smuggled into a package - are asserted as
rejections, not described in prose.

Run: python tools/contracts/validate_packaging.py
"""

from __future__ import annotations

import json
import pathlib
import sys

from jsonschema import Draft202012Validator

PACKAGING_DIR = pathlib.Path(__file__).resolve().parents[2] / "contracts" / "packaging"

MANIFEST = "download-manifest.schema.json"
PROFILE = "packaging-profile.schema.json"
TEST_CASE = "portable-test-case.schema.json"


def _manifest(**overrides: object) -> dict:
    """A minimal valid manifest, so a counterexample shows only its own defect."""
    base = {
        "schema_version": "1.0",
        "packaged_at": "2026-08-17T09:00:00Z",
        "packager_version": "0.1.0",
        "profile_id": "standard",
        "profile_version": "1.0.0",
        "source": {
            "skill_id": "11111111-1111-4111-8111-111111111111",
            "skill_version_id": "22222222-2222-4222-8222-222222222222",
            "version_number": 1,
            "content_hash": "5d41402abc4b2a76b9719d911017c592ab6f1e2d3c4b5a69788796a5b4c3d2e1",
            "origin": {
                "kind": "import",
                "source_type": "upload",
                "source_url": None,
                "source_ref": None,
                "fetched_at": "2026-07-01T12:00:00Z",
                "content_hash": "c3fcd3d76192e4007dfb496cca67e13b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f",
            },
        },
        "license": {"expression": "MIT", "source_tier": "manifest", "disclosures": []},
        "validation": {"blocked": False, "errors": [], "warnings": [], "infos": []},
        "compatibility": {
            "format": "valid",
            "capability": "unverified",
            "behaviour": "unverified",
        },
        "included_test_cases": [],
        "excluded_test_cases": [],
        "manifest_hash": "0a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f9",
    }
    base.update(overrides)
    return base


def _profile(_drop: tuple[str, ...] = (), **overrides: object) -> dict:
    base = {
        "schema_version": "1.0",
        "id": "claude-code",
        "kind": "profile",
        "version": "1.0.0",
        "display_name": "Claude Code",
        "support_status": "verified",
        "install": {
            "locations": [
                {
                    "scope": "user",
                    "path": "~/.claude/skills/<name>/",
                    "description": "Available in every project for this user.",
                }
            ],
            "top_level_dir": "<name>",
        },
        "frontmatter_additions": {},
        "env_vars": [],
        "verification_prompt": "List the skills you can use.",
        "known_limitations": [],
    }
    base.update(overrides)
    for key in _drop:
        base.pop(key, None)
    return base


def _test_case(**overrides: object) -> dict:
    base = {
        "schema_version": "1.0",
        "slug": "monthly-report",
        "name": "Monthly report",
        "origin": "curated",
        "user_prompt": "Summarise data/ledger.csv.",
        "criteria": [{"id": "c1", "text": "The total is correct."}],
        "datasets": [],
    }
    base.update(overrides)
    return base


# One counterexample per failure mode that actually matters. Each must be rejected.
NEGATIVE_CASES: list[tuple[str, str, dict]] = [
    (
        MANIFEST,
        "a licence expression with no provenance tier (ADR-021 decision 1)",
        _manifest(license={"expression": "MIT", "source_tier": None, "disclosures": []}),
    ),
    (
        MANIFEST,
        "a provenance tier with no licence expression (the same pairing, other way)",
        _manifest(license={"expression": None, "source_tier": "manifest", "disclosures": []}),
    ),
    (
        MANIFEST,
        "NOASSERTION written into license.expression (ADR-021 decision 7)",
        _manifest(
            license={"expression": "NOASSERTION", "source_tier": "manifest", "disclosures": []}
        ),
    ),
    (
        MANIFEST,
        "a behaviour verdict without the image it was measured on (04 group-B item 4)",
        _manifest(
            compatibility={
                "format": "valid",
                "capability": "unverified",
                # A measured verdict extrapolated off its (version x image) row is
                # exactly what changing the runtime image is supposed to invalidate.
                "behaviour": "native",
            }
        ),
    ),
    (
        MANIFEST,
        "evaluation prose smuggled into the improvement provenance (iron rule 11)",
        _manifest(
            source={
                "skill_id": "11111111-1111-4111-8111-111111111111",
                "skill_version_id": "22222222-2222-4222-8222-222222222222",
                "version_number": 2,
                "content_hash": (
                    "5d41402abc4b2a76b9719d911017c592ab6f1e2d3c4b5a69788796a5b4c3d2e1"
                ),
                "origin": {
                    "kind": "improvement",
                    "evaluation_id": "66666666-6666-4666-8666-666666666666",
                    "suggestions": [
                        {
                            "category": "skill",
                            "target_path": "SKILL.md",
                            # Model-written prose quoting the Run's private inputs.
                            "problem": "The skill ignored the user's spreadsheet.",
                            "expected_impact": "Fewer missed columns.",
                        }
                    ],
                    "base": "unavailable",
                    "root_source": "unavailable",
                },
            }
        ),
    ),
    (
        MANIFEST,
        "a manifest for a package that failed revalidation (PACK-002)",
        _manifest(
            validation={
                "blocked": True,
                "errors": [{"code": "possible-secret", "message": "credential pattern found"}],
                "warnings": [],
                "infos": [],
            }
        ),
    ),
    (
        MANIFEST,
        "a fork origin that records one hop and no original source (DISC-003 5)",
        _manifest(
            source={
                "skill_id": "11111111-1111-4111-8111-111111111111",
                "skill_version_id": "22222222-2222-4222-8222-222222222222",
                "version_number": 1,
                "content_hash": (
                    "5d41402abc4b2a76b9719d911017c592ab6f1e2d3c4b5a69788796a5b4c3d2e1"
                ),
                "origin": {
                    "kind": "fork",
                    "upstream_skill_id": "33333333-3333-4333-8333-333333333333",
                    "upstream_skill_version_id": "44444444-4444-4444-8444-444444444444",
                    "chain": [
                        {
                            "skill_id": "33333333-3333-4333-8333-333333333333",
                            "skill_version_id": "44444444-4444-4444-8444-444444444444",
                        }
                    ],
                },
            }
        ),
    ),
    (
        PROFILE,
        "the standard package adding frontmatter to SKILL.md (PDM-008)",
        _profile(
            id="standard",
            kind="standard_package",
            install={"locations": [], "top_level_dir": None},
            frontmatter_additions={"agent": "claude-code"},
        ),
    ),
    (
        PROFILE,
        "a profile overwriting an existing frontmatter field (ADR-012)",
        _profile(frontmatter_additions={"description": "rewritten by the adapter"}),
    ),
    (
        PROFILE,
        "an Agent SDK profile whose snippet omits setting_sources (ADR-023)",
        _profile(
            id="claude-agent-sdk",
            install={
                "locations": [
                    {
                        "scope": "agent_working_dir",
                        "path": ".claude/skills/<name>/",
                        "description": "Relative to the SDK's cwd.",
                    }
                ],
                "top_level_dir": "<name>",
            },
            snippet='query(prompt="hi", options=ClaudeAgentOptions(cwd="./workdir"))',
        ),
    ),
    (
        PROFILE,
        "a target with neither a verification prompt nor a check step (PACK-007)",
        _profile(_drop=("verification_prompt",)),
    ),
    (
        PROFILE,
        "a real-looking API key as an env var example (iron rule 11)",
        _profile(
            env_vars=[
                {
                    "name": "ANTHROPIC_API_KEY",
                    "required": True,
                    "description": "SDK credential.",
                    # Split so the literal credential prefix never appears in the
                    # tree: the repository's own pre-push scan greps for exactly
                    # this pattern, and a counterexample is not worth a false
                    # positive on every future scan.
                    "example": "sk-" + "ant-api03-DEADBEEF",
                }
            ]
        ),
    ),
    (
        TEST_CASE,
        "a dataset path that escapes on extraction (packaging-design 4.4)",
        _test_case(
            datasets=[
                {
                    "file": "data/../../.ssh/id_rsa",
                    "sha256": (
                        "c3fcd3d76192e4007dfb496cca67e13b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f"
                    ),
                    "content_type": "text/plain",
                }
            ]
        ),
    ),
    (
        TEST_CASE,
        "a user-uploaded Test Case declaring itself portable (packaging-design 5.1)",
        _test_case(origin="user_upload"),
    ),
    (
        TEST_CASE,
        "a rubric item with no evidence_required (0026 CHECK, outward form)",
        _test_case(
            rubric={
                "version": "v1",
                "items": [{"id": "c1", "text": "The total is quoted."}],
            }
        ),
    ),
]

# The canonical manifest-hash preimage is a $defs sub-schema rather than a file of
# its own, so it needs its own pair of assertions: the definition is only useful if
# it actually refuses the two inputs that would make the hash answer the wrong
# question (packaging-design.md 2.4).
HASH_INPUT_NEGATIVE_CASES: list[tuple[str, dict]] = [
    (
        "the manifest hashing itself",
        {
            "SKILL.md": "5d41402abc4b2a76b9719d911017c592ab6f1e2d3c4b5a69788796a5b4c3d2e1",
            "skillhub-manifest.json": (
                "0a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f9"
            ),
        },
    ),
    (
        "a path that escapes on extraction",
        {
            "../outside.txt": "5d41402abc4b2a76b9719d911017c592ab6f1e2d3c4b5a69788796a5b4c3d2e1"
        },
    ),
    (
        "a value that is not a sha256 digest",
        {"SKILL.md": "not-a-hash"},
    ),
]


def main() -> int:
    failures = 0
    examples_total = 0
    validators: dict[str, Draft202012Validator] = {}

    for name in (MANIFEST, PROFILE, TEST_CASE):
        schema = json.loads((PACKAGING_DIR / name).read_text(encoding="utf-8"))
        Draft202012Validator.check_schema(schema)
        validator = Draft202012Validator(
            schema, format_checker=Draft202012Validator.FORMAT_CHECKER
        )
        validators[name] = validator

        examples = schema.get("examples", [])
        if not examples:
            print(f"FAIL  {name} carries no examples")
            failures += 1
        examples_total += len(examples)
        for index, example in enumerate(examples):
            errors = sorted(validator.iter_errors(example), key=lambda e: e.path)
            if errors:
                failures += 1
                print(f"FAIL  example {name}[{index}]")
                for err in errors:
                    print(f"        /{'/'.join(map(str, err.path))}: {err.message}")
            else:
                print(f"ok    example {name}[{index}]")

        if name == MANIFEST:
            hash_schema = schema["$defs"]["manifestHashInput"]
            hash_validator = Draft202012Validator(hash_schema)
            for index, example in enumerate(hash_schema.get("examples", [])):
                examples_total += 1
                errors = sorted(hash_validator.iter_errors(example), key=lambda e: e.path)
                if errors:
                    failures += 1
                    print(f"FAIL  example manifestHashInput[{index}]")
                    for err in errors:
                        print(f"        /{'/'.join(map(str, err.path))}: {err.message}")
                else:
                    print(f"ok    example manifestHashInput[{index}]")
            for label, case in HASH_INPUT_NEGATIVE_CASES:
                if hash_validator.is_valid(case):
                    failures += 1
                    print(f"FAIL  counterexample accepted: {label}")
                else:
                    print(f"ok    counterexample rejected: {label}")

    for schema_name, label, case in NEGATIVE_CASES:
        if validators[schema_name].is_valid(case):
            failures += 1
            print(f"FAIL  counterexample accepted: {label}")
        else:
            print(f"ok    counterexample rejected: {label}")

    negatives = len(NEGATIVE_CASES) + len(HASH_INPUT_NEGATIVE_CASES)
    print(
        f"\n3 schemas, {examples_total} examples, {negatives} counterexamples, "
        f"{failures} failure(s)"
    )
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
