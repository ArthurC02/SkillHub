#!/usr/bin/env python3
import sys

import check_egress_allowlist as chk

GATEWAY = {"name": "model_gateway", "tier": "sandbox", "fqdn": "litellm.internal",
           "pinned_ip": "10.1.2.3", "port": 4000}


def test_every_finding_of_a_mixed_list_comes_back_in_order():
    entries = [
        {"name": "odd", "tier": "edge", "fqdn": "api.openai.com"},
        dict(GATEWAY, pinned_ip="unset", port=None),
        {"name": "extra", "tier": "sandbox", "fqdn": "x.internal", "pinned_ip": "", "port": 1},
        {"name": "docs", "tier": "node", "fqdn": "svc.internal", "pinned_ip": "10.0.0.1", "port": 0},
        {"name": "plain", "tier": "node", "fqdn": "docs.example.com"},
    ]
    errors, warnings = chk.check(entries)
    assert errors == [
        "odd: tier must be 'sandbox' or 'node'",
        "odd: api.openai.com is a model provider domain. N-07 has no exception path — add the "
        "provider inside the LiteLLM gateway instead (iron rule 8).",
        "tier:sandbox must hold exactly one entry named model_gateway, found "
        "['model_gateway', 'extra']. **the sandbox egress reevaluation condition has been "
        "triggered** — the allow-list is no longer 'one platform-owned destination', so the L7 "
        "proxy decision (Squid) must be re-opened before this lands.",
        "model_gateway: port must be an integer 1-65535, got None. The sandbox rule accepts to "
        "pinned_ip:port — without a port there is no rule to render, and T5-7 (probing other "
        "ports on the pinned IP) is testing nothing",
        "extra: tier:sandbox requires pinned_ip",
        "docs: port must be an integer 1-65535, got 0. The sandbox rule accepts to "
        "pinned_ip:port — without a port there is no rule to render, and T5-7 (probing other "
        "ports on the pinned IP) is testing nothing",
    ], errors
    assert warnings == [
        "model_gateway: pinned_ip is 'unset' — fail-closed, no sandbox node built from this "
        "file can reach any destination",
        "docs: tier:node must not pin an IP",
        "docs: platform-owned host on tier:node — should it be tier:sandbox?",
    ], warnings


def test_a_valid_pin_yields_no_findings_and_a_node_without_port_is_fine():
    errors, warnings = chk.check([GATEWAY, {"name": "d", "tier": "node", "fqdn": "a.example"}])
    assert (errors, warnings) == ([], []), (errors, warnings)


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
