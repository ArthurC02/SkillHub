#!/usr/bin/env python3
"""Teeth for check_egress_allowlist.py.

Plain asserts, no fixtures: runnable as `python3 tools/ci/test_check_egress_allowlist.py`
(what the workflow can call — it only pip-installs PyYAML) and collected by pytest
(`test_*.py`, matching apps/llm/tests) when someone happens to have it.
"""
import sys

import check_egress_allowlist as chk


def _entry(**over):
    e = {
        "name": "model_gateway", "tier": "sandbox", "fqdn": "litellm.internal",
        "pinned_ip": "10.1.2.3", "port": 4000,
    }
    e.update(over)
    return e


def _rejects(reason, **over):
    errors, _ = chk.check([_entry(**over)])
    assert errors, f"{reason} was accepted: {over}"
    return errors


def test_valid_entry_passes():
    errors, _ = chk.check([_entry()])
    assert errors == [], errors


def test_cidr_pin_is_rejected():
    for cidr in ("0.0.0.0/0", "10.0.0.0/8", "10.1.2.3/32"):
        _rejects("a CIDR pinned_ip", pinned_ip=cidr)


def test_hostname_pin_is_rejected():
    _rejects("a hostname pinned_ip", pinned_ip="litellm.internal")


def test_missing_port_is_rejected():
    e = _entry()
    del e["port"]
    errors, _ = chk.check([e])
    assert errors, "a sandbox entry with no port was accepted"
    _rejects("a non-numeric port", port="4000")
    _rejects("an out-of-range port", port=70000)


def test_ipv6_pin_is_rejected():
    # The file is IPv4-only by decision; a v6 pin renders nothing (see the module
    # docstring) and must not read as coverage.
    _rejects("an IPv6 pinned_ip", pinned_ip="2001:db8::1")


def test_loopback_pin_is_rejected():
    _rejects("a loopback pinned_ip", pinned_ip="127.0.0.1")
    _rejects("the unspecified address as pinned_ip", pinned_ip="0.0.0.0")


def test_unset_stays_fail_closed_not_an_error():
    errors, warnings = chk.check([_entry(pinned_ip="unset")])
    assert errors == [], errors
    assert any("fail-closed" in w for w in warnings), warnings


def test_real_allowlist_passes():
    assert chk.main() == 0, "the committed infra/egress/allowlist.yaml no longer passes"


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
