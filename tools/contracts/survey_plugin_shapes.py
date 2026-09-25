#!/usr/bin/env python3
"""Count which plugin manifest shape published repositories actually ship.

The import design turns on one measurable claim: how many sources carry a
manifest conforming to the Agent Plugins specification, against how many carry
a host-specific one instead. This answers it from GitHub, anonymously.

Usage:
    python survey_plugin_shapes.py                  # summary
    python survey_plugin_shapes.py --json out.json  # also keep the rows

The sample is every repository the listed topics return, deduplicated. It is a
sample of what topics surface, not a census of the ecosystem, and the summary
prints its own size so a reader can weigh it.
"""

from __future__ import annotations

import argparse
import collections
import json
import sys
import time
import urllib.error
import urllib.request

CANONICAL_SCHEMA = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
TOPICS = ["agent-skills", "claude-skills", "claude-code-plugin", "agent-plugins", "claude-plugin"]
PROBES = ["plugin.json", ".claude-plugin/plugin.json", ".claude-plugin/marketplace.json", "SKILL.md"]
HEADERS = {"User-Agent": "skillhub-plugin-shape-survey"}


def _get(url: str, accept_json: bool):
    request = urllib.request.Request(url, headers=dict(HEADERS, Accept="application/vnd.github+json"))
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            body = response.read(200_000)
        return 200, (json.loads(body) if accept_json else body.decode("utf-8", "replace"))
    except urllib.error.HTTPError as e:
        return e.code, None
    except Exception:
        return 0, None


def discover() -> dict[str, dict]:
    repos: dict[str, dict] = {}
    for topic in TOPICS:
        status, page = _get(
            f"https://api.github.com/search/repositories?q=topic:{topic}&sort=stars&per_page=40", True)
        if status != 200:
            print(f"topic {topic}: HTTP {status}", file=sys.stderr)
            continue
        for item in page.get("items", []):
            repos.setdefault(item["full_name"], {
                "stars": item["stargazers_count"], "default_branch": item["default_branch"]})
        time.sleep(7)
    return repos


def probe(repos: dict[str, dict]) -> dict[str, dict]:
    for name, row in repos.items():
        row["found"] = []
        for path in PROBES:
            status, body = _get(
                f"https://raw.githubusercontent.com/{name}/{row['default_branch']}/{path}", False)
            if status != 200:
                continue
            row["found"].append(path)
            if path == "plugin.json":
                try:
                    manifest = json.loads(body)
                except ValueError:
                    row["root_schema"] = "(unparseable)"
                    continue
                row["root_schema"] = manifest.get("$schema", "(absent)")
                row["root_fields"] = sorted(manifest)
        time.sleep(0.05)
    return repos


def summarise(repos: dict[str, dict]) -> None:
    conforming = [n for n, r in repos.items() if r.get("root_schema") == CANONICAL_SCHEMA]
    host_specific = [n for n, r in repos.items() if ".claude-plugin/plugin.json" in r["found"]]
    marketplace = [n for n, r in repos.items() if ".claude-plugin/marketplace.json" in r["found"]]
    both = sorted(set(conforming) & set(host_specific))

    print(f"sample                                   {len(repos)}")
    print(f"conforming root plugin.json              {len(conforming)}")
    print(f"host-specific .claude-plugin/plugin.json {len(host_specific)}")
    print(f"  of those, host-specific only           {len(set(host_specific) - set(conforming))}")
    print(f"  shipping both shapes                   {len(both)}")
    print(f".claude-plugin/marketplace.json          {len(marketplace)}")

    print("\nroot plugin.json by declared $schema")
    for schema, count in collections.Counter(
            r["root_schema"] for r in repos.values() if "root_schema" in r).most_common():
        print(f"  {count:>4}  {schema}")

    print(f"\nfields present among the {len(conforming)} conforming manifests")
    fields: collections.Counter = collections.Counter()
    for name in conforming:
        fields.update(repos[name].get("root_fields", []))
    for field, count in fields.most_common():
        print(f"  {count:>4}/{len(conforming)}  {field}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__,
                                     formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--json", help="write the per-repository rows here as well")
    args = parser.parse_args()

    repos = probe(discover())
    if not repos:
        print("no repositories discovered; GitHub search may be rate limiting", file=sys.stderr)
        return 1
    summarise(repos)
    if args.json:
        with open(args.json, "w", encoding="utf-8", newline="") as f:
            json.dump(repos, f, ensure_ascii=False, indent=1)
        print(f"\nrows written to {args.json}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
