#!/usr/bin/env python3
"""Tests for render.py's egress-allow.json output: the JSON keys must stay
purpose/fqdn/pinned_ip/port/protocol under a `destinations` key, matching
apps/sandbox/internal/sandbox's Go struct tags."""
import json
import sys

import render

READER_KEYS = {"purpose", "fqdn", "pinned_ip", "port", "protocol"}
READER_TOP_LEVEL_KEY = "destinations"


def test_the_emitted_destination_matches_the_keys_the_reader_declares():
    entries = [{
        "name": "model_gateway", "tier": "sandbox", "fqdn": "litellm.internal",
        "pinned_ip": "10.1.2.3", "port": 4000, "protocol": "tcp",
    }]
    payload = json.loads(render.render_admission(entries))

    assert READER_TOP_LEVEL_KEY in payload, (
        f"the rendered admission list has no {READER_TOP_LEVEL_KEY!r} key; "
        f"sandbox/egress.go unmarshals that field and would see zero destinations. "
        f"Got: {sorted(payload)}")

    dests = payload[READER_TOP_LEVEL_KEY]
    assert len(dests) == 1, (
        f"one pinned sandbox-tier entry must render exactly one destination, got {dests!r}. "
        f"An empty list here is why this assertion exists: the committed file is "
        f"legitimately empty today, so a test that only read it would compare 0 with 0.")

    assert set(dests[0]) == READER_KEYS, (
        f"the rendered destination's keys are {sorted(dests[0])}, but "
        f"apps/sandbox/internal/sandbox/egress_test.go pins the reader to "
        f"{sorted(READER_KEYS)}. Renaming a key on either side alone makes the node's "
        f"admission list unreadable, and the symptom is a node that refuses every run "
        f"naming a destination -- indistinguishable from fail-closed working correctly.")


def test_a_node_tier_entry_recurses_by_name_and_carries_no_pin():
    entries = [{"name": "docs-site", "tier": "node", "fqdn": "docs.example.com"}]
    dns = render.render_dnsmasq(entries, "10.0.0.53")

    assert "server=/docs.example.com/#" in dns, (
        f"a tier:node entry must recurse by fqdn, not be pinned: {dns!r}")
    assert "address=/docs.example.com/" not in dns, (
        f"a tier:node entry must not get the sandbox-tier pinned address line: {dns!r}")


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
