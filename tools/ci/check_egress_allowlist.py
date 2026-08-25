#!/usr/bin/env python3
"""Assert ADR-022 Q3's invariants on infra/egress/allowlist.yaml.

Run it anywhere: `python3 tools/ci/check_egress_allowlist.py`.
Exit 0 = the allow-list still matches the decision; exit 1 = it does not, and the
message says which part of ADR-022 to go read.

Address family — this allow list is IPv4-only, and that is a decision, not an
oversight. Every `pinned_ip` is an A record and renders into the `ip` table. The
renderer MUST therefore leave the `ip6` table at `policy drop` with no accept
exceptions, and the node resolver must not answer AAAA for allow-list names.
Reason: ADR-022's T5 acceptance suite is entirely IPv4-shaped (169.254.169.254,
RFC1918 /24 scans, `pinned_ip` port probes), so a node whose `ip` table is locked
down while its `ip6` table sits at `policy accept` passes all eight sub-items and
still has a wide-open egress path off the sandbox. T5 cannot see that hole, so the
file closes it by construction instead: no v6 pin can be written here at all.
Making the file dual-stack (a v6 pin per entry, plus a v6 half of T5) is the other
honest option and is a bigger change than one platform-owned destination warrants.
"""
import ipaddress
import pathlib
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[2]
PATH = ROOT / "infra" / "egress" / "allowlist.yaml"

# N-07 has no exception path. Substring match on the fqdn, not equality: a
# regional or versioned host of the same provider is the same bypass.
PROVIDER_DOMAINS = (
    "openai.com", "anthropic.com", "googleapis.com", "google.com",
    "azure.com", "mistral.ai", "cohere.com", "bedrock", "x.ai",
)


def _port_error(e):
    """The accept rule's unit is IP:port, not IP (T5-7). A missing port is a rule
    nobody can render, and a bool is an int in Python — reject it explicitly."""
    port = e.get("port")
    if isinstance(port, bool) or not isinstance(port, int) or not 1 <= port <= 65535:
        return (
            f"{e.get('name')}: port must be an integer 1-65535, got {port!r}. The "
            f"sandbox rule accepts to pinned_ip:port — without a port there is no "
            f"rule to render, and T5-7 (probing other ports on the pinned IP) is "
            f"testing nothing (ADR-022 Q3)"
        )
    return None


def _pin_errors(e, pin):
    """`pinned_ip` must be one host address. ip_address() rejects a CIDR and a
    hostname for free; the rest is what a syntactically valid pin still must not be."""
    name = e.get("name")
    try:
        addr = ipaddress.ip_address(pin)
    except ValueError:
        return [
            f"{name}: pinned_ip must be a single host address, got {pin!r}. A CIDR "
            f"or a hostname is not a pin — `pinned_ip: 10.0.0.0/8` renders an accept "
            f"rule the size of the network, and a name renders whatever the resolver "
            f"says today (ADR-022 Q3: the sandbox tier pins IP:port precisely because "
            f"the destination is platform-owned)"
        ]
    if addr.version != 4:
        return [
            f"{name}: pinned_ip {pin} is IPv6. This allow list is IPv4-only by "
            f"decision — the renderer keeps the `ip6` table at policy drop with no "
            f"exceptions, so a v6 pin here renders nothing and reads as coverage that "
            f"does not exist. See this file's module docstring and allowlist.yaml's "
            f"address-family note before changing it; going dual-stack means growing "
            f"ADR-022's T5 suite a v6 half too"
        ]
    if addr.is_loopback or addr.is_unspecified or addr.is_multicast:
        return [
            f"{name}: pinned_ip {pin} is not a reachable unicast destination "
            f"(loopback/unspecified/multicast). Q2 constraint 3 says the sandbox must "
            f"not reach the node's loopback at all (ADR-022 Q2, T5-5)"
        ]
    return []


def check(entries):
    """Return (errors, warnings). Split out from main so the assertions are testable
    without a real allow-list on disk."""
    errors, warnings = [], []

    sandbox = [e for e in entries if e.get("tier") == "sandbox"]
    node = [e for e in entries if e.get("tier") == "node"]

    for e in entries:
        if e.get("tier") not in ("sandbox", "node"):
            errors.append(f"{e.get('name')}: tier must be 'sandbox' or 'node' (ADR-022 Q3)")
        fqdn = str(e.get("fqdn", "")).lower()
        for bad in PROVIDER_DOMAINS:
            if bad in fqdn:
                errors.append(
                    f"{e.get('name')}: {fqdn} is a model provider domain. N-07 has no "
                    f"exception path — add the provider inside the LiteLLM gateway "
                    f"instead (iron rule 8)."
                )

    # The re-evaluation condition, mechanised. One platform-owned destination is what
    # made "no L7 proxy" the right call; a second one is the trigger to re-open it.
    if len(sandbox) != 1 or sandbox[0].get("name") != "model_gateway":
        errors.append(
            f"tier:sandbox must hold exactly one entry named model_gateway, found "
            f"{[e.get('name') for e in sandbox]}. **ADR-022 Q3 的重評條件已觸發** — the "
            f"allow-list is no longer 'one platform-owned destination', so the L7 proxy "
            f"decision (Squid) must be re-opened before this lands."
        )

    for e in sandbox:
        pin = str(e.get("pinned_ip", "")).strip()
        if not pin:
            errors.append(f"{e.get('name')}: tier:sandbox requires pinned_ip (ADR-022 Q3)")
        elif pin == "unset":
            # Not an error — unset renders no accept rule, which is the fail-closed
            # direction. But it means no node built from this file can reach anything,
            # so it has to be visible on every run rather than discovered at deploy time.
            warnings.append(
                f"{e.get('name')}: pinned_ip is 'unset' — fail-closed, no sandbox node "
                f"built from this file can reach any destination (ADR-022 Q3)"
            )
        else:
            errors.extend(_pin_errors(e, pin))
        # Required on the sandbox tier: `unset` withholds the address, never the port.
        err = _port_error(e)
        if err:
            errors.append(err)

    # Node tier talks to third parties whose IPs rotate; a pin there is a rule that
    # breaks silently on the vendor's next deploy.
    for e in node:
        if e.get("pinned_ip"):
            warnings.append(f"{e.get('name')}: tier:node must not pin an IP (ADR-022 A1-a)")
        if str(e.get("fqdn", "")).endswith(".internal"):
            warnings.append(f"{e.get('name')}: platform-owned host on tier:node — should it be tier:sandbox?")
        # Not required here: the node tier matches on FQDN, not IP:port (ADR-022 Q3),
        # so an entry without a port is legitimate. A malformed one never is.
        if e.get("port") is not None and _port_error(e):
            errors.append(_port_error(e))

    return errors, warnings


def main(path=PATH):
    entries = yaml.safe_load(path.read_text(encoding="utf-8"))["destinations"] or []
    errors, warnings = check(entries)
    for w in warnings:
        print(f"::warning::{w}")
    for err in errors:
        print(f"::error::{err}")
    sandbox = sum(1 for e in entries if e.get("tier") == "sandbox")
    node = sum(1 for e in entries if e.get("tier") == "node")
    print(f"checked {len(entries)} destinations: {sandbox} sandbox, {node} node")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
