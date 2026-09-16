#!/usr/bin/env python3
"""Record the curation review of the seed list's curated skills.

Drives the public HTTP API only: the catalogue owner's session lists the
catalogue's skills, and an operator session records each review through
PUT /admin/skills/{id}/tier, which binds it to the skill's newest version.

Usage:
    python curate_seed.py --dry-run
    python curate_seed.py --user seed-importer --operator seed-operator
"""

from __future__ import annotations

import argparse
import importlib.util
import json
import pathlib
import sys

HERE = pathlib.Path(__file__).parent
SEED = HERE / "seed-skills.json"
NOTE = "PDM-002 nine-item review passed; see docs/plans/mvp/content/curated-skill-list.md"


def _load(name: str):
    spec = importlib.util.spec_from_file_location(name, HERE / f"{name}.py")
    if spec is None or spec.loader is None:
        raise SystemExit(f"cannot load {name}.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def curated_names() -> list[str]:
    seed = json.loads(SEED.read_text(encoding="utf-8"))
    return [s["name"] for s in seed["skills"] if s["tier"] == "curated"]


def run(args) -> int:
    importer, testcases = _load("import_seed"), _load("seed_testcases")
    owner = testcases.Client(importer, args.api)
    importer.dev_login(owner.opener, args.api, args.user)
    operator = owner
    if args.operator != args.user:
        operator = testcases.Client(importer, args.api)
        importer.dev_login(operator.opener, args.api, args.operator)

    skills = {s["name"]: s["skill_id"] for s in owner.call("GET", "/skills")["skills"]}
    absent = 0
    for name in curated_names():
        skill_id = skills.get(name)
        if skill_id is None:
            absent += 1
            print(f"  {name:<28} skill_absent (run import_seed.py as {args.user} first)")
            continue
        if args.dry_run:
            print(f"  {name:<28} would_curate ({skill_id})")
            continue
        body = operator.call("PUT", f"/admin/skills/{skill_id}/tier",
                             json_body={"value": "curated", "note": NOTE})
        print(f"  {name:<28} {body['tier']} on {body['curated_version_id']} (was {body['previous_tier']})")
    return 1 if absent else 0


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--api", default=_load("import_seed").DEFAULT_API)
    p.add_argument("--user", default="seed-importer",
                   help="dev-login user; must own the workspace with is_catalog = true")
    p.add_argument("--operator", default="seed-importer",
                   help="dev-login user listed in OPERATOR_USER_IDS")
    p.add_argument("--dry-run", action="store_true")
    return run(p.parse_args())


if __name__ == "__main__":
    sys.exit(main())
