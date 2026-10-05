#!/usr/bin/env python3
import sys

import checkout as co

FILES = {
    "infra/deploy/agent/checkout-paths",
    "infra/deploy/agent/run.mjs",
    "infra/compose/agent.yml",
    "infra/deploy/other/run.mjs",
}

PATTERNS_TEXT = "infra/deploy/agent/\ninfra/compose/agent.yml\ninfra/deploy/agent/nonexistent-dir/\n"

SOURCES = {
    "infra/deploy/agent/checkout-paths": PATTERNS_TEXT,
    "infra/deploy/agent/run.mjs": "",
    "infra/compose/agent.yml": "/opt/skillhub/infra/deploy/agent/compose-only-ref.sh",
    co.TEMPLATE: (
        "run /opt/skillhub/infra/deploy/agent/run.mjs "
        "and /opt/skillhub/infra/deploy/agent/absent.sh"
    ),
}


def read(source):
    return SOURCES[source]


def test_a_reference_the_checkout_carries_names_no_problem():
    problems, node = co.gaps("agent", FILES, read)
    assert node == {
        "infra/deploy/agent/checkout-paths",
        "infra/deploy/agent/run.mjs",
        "infra/compose/agent.yml",
    }
    assert problems == [
        "checkout-paths entry 'infra/deploy/agent/nonexistent-dir/' selects no file",
        "infra/deploy/cloud-init.yaml.tmpl names infra/deploy/agent/absent.sh, "
        "which the agent checkout does not carry",
        "infra/compose/agent.yml names infra/deploy/agent/compose-only-ref.sh, "
        "which the agent checkout does not carry",
    ], problems


def test_every_source_that_names_an_uncarried_reference_is_reported_once_each():
    sources = dict(SOURCES)
    sources["infra/deploy/agent/run.mjs"] = "/opt/skillhub/infra/deploy/agent/also-absent.sh"
    problems, _ = co.gaps("agent", FILES, lambda source: sources[source])
    assert problems == [
        "checkout-paths entry 'infra/deploy/agent/nonexistent-dir/' selects no file",
        "infra/deploy/cloud-init.yaml.tmpl names infra/deploy/agent/absent.sh, "
        "which the agent checkout does not carry",
        "infra/compose/agent.yml names infra/deploy/agent/compose-only-ref.sh, "
        "which the agent checkout does not carry",
        "infra/deploy/agent/run.mjs names infra/deploy/agent/also-absent.sh, "
        "which the agent checkout does not carry",
    ], problems


if __name__ == "__main__":
    failed = 0
    for name, fn in sorted(globals().items()):
        if name.startswith("test_") and callable(fn):
            try:
                fn()
                print(f"ok   {name}")
            except AssertionError as exc:
                failed += 1
                print(f"FAIL {name}: {exc}")
    sys.exit(1 if failed else 0)
