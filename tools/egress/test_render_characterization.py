#!/usr/bin/env python3
import contextlib
import hashlib
import io
import sys

import render

PINNED = [{"name": "model_gateway", "tier": "sandbox", "fqdn": "gw.internal",
           "pinned_ip": "10.9.9.9", "port": 4000, "protocol": "tcp"}]
UNSET = [{"name": "model_gateway", "tier": "sandbox", "fqdn": "gw.internal",
          "pinned_ip": "unset", "port": 4000}]


def digest(text):
    return hashlib.sha256(text.encode()).hexdigest()


def test_a_pin_with_resolver_and_control_plane_renders_the_same_bytes():
    got = digest(render.render_nftables(PINNED, "sbx0", "10.0.0.53", "10.1.1.1"))
    assert got == "32f55191825c3b4e8914c5f8a9d0234ccf7f9b4c63550522dd03932ab5f9c35a", got


def test_an_unset_pin_without_resolver_or_control_plane_renders_the_same_bytes():
    got = digest(render.render_nftables(UNSET, "skillhub-sbx", "", ""))
    assert got == "08a98311a38fd9667a2d6c4c8b84c1ecd24563e1d28335d90ffbf829dcde3c60", got


def test_a_pin_with_only_a_control_plane_renders_the_same_bytes():
    got = digest(render.render_nftables(PINNED, "docker0", "", "10.0.0.2"))
    assert got == "d49125356a0dc5b5559f58ce73c1d090dbab9d9a7316c1ac9e7ab697fb412405", got


def test_the_chains_appear_in_their_load_bearing_order():
    lines = render.render_nftables(PINNED, "sbx0", "10.0.0.53", "10.1.1.1").splitlines()
    order = [lines.index(marker) for marker in (
        "delete table ip6 skillhub",
        "table ip skillhub {",
        "    chain forward {",
        "    chain input {",
        "table ip6 skillhub {",
    )]
    assert order == sorted(order), order
    assert lines[-1] == "}", lines[-1]


def test_the_self_check_passes_offline():
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = render.self_check()
    assert code == 0
    lines = out.getvalue().splitlines()
    assert lines[-1].startswith("self-check: "), lines[-1]
    assert "FAIL" not in out.getvalue(), out.getvalue()
    assert sum(1 for line in lines if line.endswith(" ok")) == 18, out.getvalue()
    assert lines[-1] == "self-check: 18/18", lines[-1]


def test_addr_rejects_an_ipv6_address():
    try:
        render._addr("2001:db8::1", "--resolver")
    except SystemExit as exc:
        assert str(exc) == "--resolver must be IPv4: this allow list is IPv4-only by decision (N-08)", exc
    else:
        raise AssertionError("an IPv6 address must be refused")


def test_addr_rejects_a_malformed_value_and_keeps_its_cause():
    try:
        render._addr("not-an-ip", "--resolver")
    except SystemExit as exc:
        assert str(exc) == "--resolver must be a single IPv4 host address, got 'not-an-ip'", exc
        assert isinstance(exc.__cause__, ValueError)
    else:
        raise AssertionError("a malformed address must be refused")


def test_addr_treats_an_empty_value_as_absent():
    assert render._addr("", "--resolver") == ""


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
