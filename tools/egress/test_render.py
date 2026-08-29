#!/usr/bin/env python3
"""Teeth for the WRITER half of infra/egress/rendered/egress-allow.json.

apps/sandbox/internal/sandbox/egress_test.go pins the READER: the Go struct's
json tags must be exactly purpose/fqdn/pinned_ip/port/protocol, and the file must
carry a `destinations` key. Nothing pinned this side, and the two sides are one
hand-written contract across two languages -- so renaming a key here left every
test in the repository green while the node's admission list stopped being
readable by the only program that reads it. The failure does not look like a
failure: sandboxd sees zero destinations and refuses every run that names one
(capability_mismatch), which reads as a node that reaches nothing, which is also
exactly what the fail-closed path looks like when it is working.

A separate file rather than two more cases in `render.py --self-check`, for the
reason automation_check_test.go gives about its own roster: a key rename and its
excuse should not be able to arrive in one edit to one file.

Plain asserts, no fixtures: runnable as `python3 tools/egress/test_render.py`
(what the workflow calls -- it only pip-installs PyYAML) and collected by pytest
when someone happens to have it. Same shape as tools/ci/test_check_egress_allowlist.py.
"""
import json
import sys

import render

# The set the Go reader declares, written out rather than imported: a constant
# both sides derive from one place would be the fix, and there is no such place
# across a module boundary this deliberate (ADR-019 iron rule 2 keeps the two Go
# modules from importing each other, and this end is not Go at all). So it is
# spelled twice on purpose, and this is the assertion that the two spellings
# agree.
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
