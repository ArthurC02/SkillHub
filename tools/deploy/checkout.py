#!/usr/bin/env python3
import argparse
import pathlib
import re
import shutil
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
TEMPLATE = "infra/deploy/cloud-init.yaml.tmpl"
REFERENCE = re.compile(r"(?:/opt/skillhub\}?/|\$here\"?/)([A-Za-z0-9_./$*-]+)")
HERE = "infra/deploy/"


def repository_files(root):
    listed = subprocess.run(["git", "-C", str(root), "ls-files", "--cached", "--others", "--exclude-standard"],
                            check=True, capture_output=True, text=True).stdout
    return {path for path in listed.splitlines() if (root / path).is_file()}


def checkout_patterns(text):
    return [line.strip() for line in text.splitlines() if line.strip()]


def selects(pattern, path):
    return path.startswith(pattern) if pattern.endswith("/") else path == pattern


def staged(patterns, files):
    return {path for path in files if any(selects(pattern, path) for pattern in patterns)}


def references(text, role):
    found = set()
    for match in REFERENCE.finditer(text):
        prefix = HERE if match.group(0).startswith("$here") else ""
        path = (prefix + match.group(1)).replace("$SKILLHUB_ROLE", role)
        found.add(path.split("*", 1)[0].rstrip("/"))
    return found


def covered(reference, files):
    return reference in files or any(path.startswith(reference + "/") for path in files)


def gaps(role, files, read):
    patterns = checkout_patterns(read("infra/deploy/%s/checkout-paths" % role))
    problems = ["checkout-paths entry %r selects no file" % pattern
                for pattern in patterns if not any(selects(pattern, path) for path in files)]
    node = staged(patterns, files)
    scanned = sorted(path for path in node if path.startswith(HERE) or path.startswith("infra/compose/"))
    for source in [TEMPLATE] + scanned:
        for reference in sorted(references(read(source), role)):
            if not covered(reference, node):
                problems.append("%s names %s, which the %s checkout does not carry" % (source, reference, role))
    return problems, node


def main(argv=None):
    parser = argparse.ArgumentParser(description="Check that a node's sparse checkout carries every file it runs.")
    parser.add_argument("role")
    parser.add_argument("--stage", type=pathlib.Path, help="also copy the checkout into this directory")
    args = parser.parse_args(argv)
    problems, node = gaps(args.role, repository_files(ROOT), lambda path: (ROOT / path).read_text(encoding="utf-8"))
    for problem in problems:
        print("checkout: %s" % problem, file=sys.stderr)
    if problems:
        return 1
    if args.stage:
        for path in sorted(node):
            target = args.stage / path
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(ROOT / path, target)
    print("checkout: %s carries all %d file(s) its scripts name" % (args.role, len(node)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
