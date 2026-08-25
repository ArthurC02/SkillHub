#!/usr/bin/env python3
"""Render infra/egress/allowlist.yaml into the things that actually enforce it.

ADR-022 Q3 chose nftables default-deny plus a node-pinned resolver over an L7
proxy, and `03` SBX-007 records what was still missing afterwards: the ruleset
itself, the resolver config, and sandboxd comparing a RunRequest's `egress.allow`
against what this node actually renders (ADR-022 A1-e). All three come from one
file, and they come from it *here*, together, on purpose.

WHY ONE RENDERER AND NOT THREE ARTIFACTS MAINTAINED BY HAND

`04` 甲-3 recorded the failure this closes: `Egress.Allow[].URL` was shown in the
pre-run permission summary, written into the run record, and then dropped - no
line of code anywhere read it. The only thing enforcing anything was the
structural accident that one Docker network had one service on it. A destination
a user is shown and agrees to, with nothing holding the run to it, is narration.

Three hand-maintained artifacts would restore that gap in a slower form: an
nftables rule accepting an address the admission check has never heard of, or an
admission check accepting a destination no rule opens. The second is the worse
one, because it dispatches the run and lets it time out - the exact behaviour
ADR-022 A1-e replaces with a `capability_mismatch` refusal.

So the ruleset, the resolver config and sandboxd's admission list are three
projections of one source, and `--check` fails CI when the committed copies stop
matching it.

FAIL-CLOSED IS THE DEFAULT EVERYWHERE

Every missing input renders *less* reachability, never more:

  pinned_ip: unset      no accept rule, and the destination is left OUT of the
                        admission list too, so the node refuses the run instead
                        of accepting it and letting it time out.
  --resolver unset      no DNS accept rule and no listen address; nothing
                        resolves, so nothing is reached by name.
  --control-plane
     unset              no inbound accept; the node answers nobody. Useless and
                        safe, in that order.

Usage:
    python3 tools/egress/render.py --out infra/egress/rendered
    python3 tools/egress/render.py --check
    python3 tools/egress/render.py --self-check
"""
import argparse
import importlib.util
import ipaddress
import json
import pathlib
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[2]
ALLOWLIST = ROOT / "infra" / "egress" / "allowlist.yaml"
DEFAULT_OUT = ROOT / "infra" / "egress" / "rendered"

# The checker is the authority on what a valid allow-list is, and importing it
# rather than restating its rules is the point: a renderer with a second opinion
# about `pinned_ip` is how the ruleset and the reviewed source diverge.
_spec = importlib.util.spec_from_file_location(
    "check_egress_allowlist", ROOT / "tools" / "ci" / "check_egress_allowlist.py")
_checker = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_checker)

# RFC1918 plus the link-local range carrying the cloud metadata service. Blocked
# by address rather than by name because N-03 is about the address: a resolver
# that refuses `metadata.google.internal` does nothing about 169.254.169.254.
PRIVATE_V4 = ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16")
LINK_LOCAL_V4 = "169.254.0.0/16"


def rendered_destinations(entries):
    """The sandbox-tier destinations this node can actually reach.

    An entry whose pin is `unset` is absent rather than present-and-unreachable.
    Its accept rule does not exist, so a run allowed to name it would be
    dispatched and then fail to connect - and "accepted, then timed out" is
    precisely what ADR-022 A1-e replaces with a refusal.
    """
    out = []
    for e in entries:
        if e.get("tier") != "sandbox":
            continue
        pin = str(e.get("pinned_ip", "")).strip()
        if not pin or pin == "unset":
            continue
        out.append({
            "purpose": e["name"],
            "fqdn": str(e.get("fqdn", "")),
            "pinned_ip": pin,
            "port": int(e["port"]),
            "protocol": str(e.get("protocol", "tcp")),
        })
    return out


def render_nftables(entries, sandbox_iface, resolver, control_plane):
    """The ruleset. Enforcement lives in `forward`, never inside the container.

    SBX-007 is explicit about the hook: the enforcement point is the host's
    forward path or the run's own netns, not an `output` chain inside the
    sandbox, because anything inside the sandbox is editable by whatever escapes
    into it. A rule the attacker can delete documents an intention.

    Rule ORDER is load-bearing, and it is not the order the requirements are
    numbered in. The denies for east-west (T5-4), the node's own services (T5-5)
    and the metadata address (T5-3) come BEFORE the allow-list accepts, so a
    `pinned_ip` that is somehow a sandbox address, the bridge gateway or the node
    itself is dropped rather than opened. That ordering is what makes ADR-022 Q2
    constraint 6 - the gateway must not share an address with the control plane -
    fail closed instead of quietly granting the sandbox a route to it.

    The accepts then come BEFORE the RFC1918 drop, because the gateway is an
    internal address: N-03 blocks the private ranges as a class and the allow
    list is the one hole punched in it, at one address and one port.

    Every rule carries `counter`. ADR-022 §5 wants nftables counters as T5's
    evidence and N-06 wants the attempt recorded: a drop nobody can count is
    indistinguishable from traffic that was never sent.
    """
    lines = []
    a = lines.append
    a("#!/usr/sbin/nft -f")
    a("#")
    a("# GENERATED - do not edit. Source: infra/egress/allowlist.yaml")
    a("# Rendered by tools/egress/render.py; `render.py --check` fails CI on drift.")
    a("# A node's ruleset is a product of that file. Editing it on the node is")
    a("# drift, and the next rebuild (P-03, <= 7 days) wipes it.")
    a("")
    a("flush ruleset")
    a("")
    a('define SANDBOX_IFACE = "' + sandbox_iface + '"')
    a("")
    a("table ip skillhub {")
    a("    chain forward {")
    a("        type filter hook forward priority filter; policy drop;")
    a("")
    a("        # Replies to something this node already allowed out.")
    a("        ct state established,related counter accept")
    a("")
    a("        # T5-4 east-west. Two sandboxes on one node must not reach each")
    a("        # other: that is a cross-run path needing no escape at all.")
    a('        iifname $SANDBOX_IFACE oifname $SANDBOX_IFACE counter log prefix "skillhub-drop-eastwest " drop')
    a("")
    a("        # T5-5 the node itself: loopback and the provider port. A sandbox")
    a("        # that reaches sandboxd can dispatch runs.")
    a('        iifname $SANDBOX_IFACE ip daddr 127.0.0.0/8 counter log prefix "skillhub-drop-nodelo " drop')
    a('        iifname $SANDBOX_IFACE tcp dport 9000 counter log prefix "skillhub-drop-sandboxd " drop')
    a("")
    a("        # T5-3 / N-03 the metadata service, by address. A resolver that")
    a("        # refuses the name does nothing about the literal.")
    a('        iifname $SANDBOX_IFACE ip daddr ' + LINK_LOCAL_V4 + ' counter log prefix "skillhub-drop-metadata " drop')
    a("")
    if resolver:
        a("        # N-04 the node's pinned resolver, and only it. A sandbox that")
        a("        # reaches any other resolver picks its own answers, which is the")
        a("        # DNS-rebinding shape IP pinning exists to remove.")
        a("        iifname $SANDBOX_IFACE ip daddr " + resolver + " udp dport 53 counter accept")
        a("        iifname $SANDBOX_IFACE ip daddr " + resolver + " tcp dport 53 counter accept")
    else:
        a("        # No resolver configured, so no DNS is accepted at all. That is")
        a("        # why a node built this way reaches nothing by name.")
    a('        iifname $SANDBOX_IFACE udp dport 53 counter log prefix "skillhub-drop-dns " drop')
    a('        iifname $SANDBOX_IFACE tcp dport 53 counter log prefix "skillhub-drop-dns " drop')
    a("")
    dests = rendered_destinations(entries)
    if dests:
        a("        # The allow list. The unit is IP:port, never IP - T5-7 probes the")
        a("        # other ports on this same address and they must be dropped.")
        for d in dests:
            a("        iifname $SANDBOX_IFACE ip daddr " + d["pinned_ip"] + " " + d["protocol"]
              + " dport " + str(d["port"]) + " counter accept  # " + d["purpose"])
    else:
        a("        # No destination has a pinned address yet, so no accept rule is")
        a("        # rendered and a sandbox on this node reaches nothing. That is")
        a("        # the fail-closed direction and it is deliberate (allowlist.yaml).")
    a("")
    a("        # N-03 the private ranges as a class, after the one hole punched in")
    a("        # them above.")
    for cidr in PRIVATE_V4:
        a('        iifname $SANDBOX_IFACE ip daddr ' + cidr + ' counter log prefix "skillhub-drop-rfc1918 " drop')
    a("")
    a("        # N-01 / N-06. `policy drop` already ends it; this rule exists so")
    a("        # the attempt is counted and logged on the way, which is what N-06")
    a("        # asks for and what T5 reads as evidence.")
    a('        iifname $SANDBOX_IFACE counter log prefix "skillhub-drop-default " drop')
    a("    }")
    a("")
    a("    chain input {")
    a("        type filter hook input priority filter; policy drop;")
    a("        ct state established,related counter accept")
    a("        iif lo counter accept")
    if control_plane:
        a("        # T5-8 / N-05. Only the control plane may reach the provider")
        a("        # port; an external scan of this node finds nothing else open.")
        a("        ip saddr " + control_plane + " tcp dport 9000 counter accept")
    else:
        a("        # No control-plane source configured, so nothing may reach the")
        a("        # provider port. Useless and safe, in that order.")
    a('        counter log prefix "skillhub-drop-inbound " drop')
    a("    }")
    a("}")
    a("")
    a("# N-08. This allow list is IPv4-only by decision, so v6 carries no accept")
    a("# rule at any point. A separate table rather than an `inet` one so that")
    a("# `nft list table ip6 skillhub` answers the question directly: T5's eight")
    a("# sub-items are all IPv4-shaped, and a v6 table left at policy accept")
    a("# passes every one of them while a sandbox walks out over v6.")
    a("table ip6 skillhub {")
    for hook in ("forward", "input", "output"):
        a("    chain " + hook + " {")
        a("        type filter hook " + hook + " priority filter; policy drop;")
        a('        counter log prefix "skillhub-drop-v6 " drop')
        a("    }")
    a("}")
    return "\n".join(lines) + "\n"


def render_dnsmasq(entries, resolver):
    """The node's pinned resolver (N-04), which is also half of T5-1.

    Two behaviours matter and neither is a default: every allow-list name
    resolves to its pin and nothing else, and every other name returns NXDOMAIN
    rather than being forwarded. A resolver that forwards the unknown names is a
    DNS tunnel with extra steps, which is what T5-1 probes for.
    """
    lines = []
    a = lines.append
    a("# GENERATED - do not edit. Source: infra/egress/allowlist.yaml")
    a("# Rendered by tools/egress/render.py; `render.py --check` fails CI on drift.")
    a("")
    a("# Never consult /etc/resolv.conf or /etc/hosts: the answers this resolver")
    a("# gives are the ones rendered below, not whatever the node happens to hold.")
    a("no-resolv")
    a("no-hosts")
    a("bind-interfaces")
    if resolver:
        a("listen-address=" + resolver)
    else:
        a("# No resolver address configured, so this binds nowhere. Fail-closed.")
    a("")
    a("# N-08. The allow list carries A records only; answering AAAA for any of")
    a("# these names hands out an address the ip6 table has no accept rule for,")
    a("# and that failure looks like a network fault rather than like policy.")
    a("filter-AAAA")
    a("")
    a("# N-06: which name was asked for, and when.")
    a("log-queries")
    a("")
    a("# T5-1. Everything not named below is NXDOMAIN. `address=/#/` with no")
    a("# address is how dnsmasq says that. Forwarding the unknown names instead")
    a("# leaves a tunnel the ip table cannot see, because those queries go to the")
    a("# resolver this node runs.")
    a("address=/#/")
    a("")
    sandbox = [e for e in entries if e.get("tier") == "sandbox"]
    node = [e for e in entries if e.get("tier") == "node"]
    if sandbox:
        a("# tier: sandbox - platform-owned, pinned to one address (ADR-022 Q3).")
    for e in sandbox:
        pin = str(e.get("pinned_ip", "")).strip()
        if pin and pin != "unset":
            a("address=/" + str(e["fqdn"]) + "/" + pin)
        else:
            a("# " + str(e.get("name")) + ": pinned_ip is unset, so this name stays NXDOMAIN.")
    if node:
        a("")
        a("# tier: node - third parties with rotating IPs, so these names recurse")
        a("# normally. Reachable by the node's own services, never by a sandbox:")
        a("# the ip table has no accept rule for them.")
        for e in node:
            a("server=/" + str(e["fqdn"]) + "/#")
    return "\n".join(lines) + "\n"


def render_admission(entries):
    """What sandboxd loads to answer ADR-022 A1-e.

    Same source as the ruleset, so "this node accepts that destination" and
    "this node has a rule for that destination" cannot become two answers.
    """
    payload = {
        "source": "infra/egress/allowlist.yaml",
        "generated_by": "tools/egress/render.py",
        "note": (
            "Destinations this node has an nftables accept rule for. An entry "
            "whose pinned_ip is unset is absent on purpose: sandboxd must refuse "
            "a run naming it (capability_mismatch) rather than dispatch it to a "
            "node with no route for it (ADR-022 A1-e)."
        ),
        "destinations": rendered_destinations(entries),
    }
    return json.dumps(payload, indent=2) + "\n"


def render_all(entries, sandbox_iface, resolver, control_plane):
    return {
        "nftables.conf": render_nftables(entries, sandbox_iface, resolver, control_plane),
        "dnsmasq.conf": render_dnsmasq(entries, resolver),
        "egress-allow.json": render_admission(entries),
    }


def load_entries(path):
    entries = yaml.safe_load(path.read_text(encoding="utf-8"))["destinations"] or []
    errors, _ = _checker.check(entries)
    if errors:
        # Refusing to render is why the checker is imported. A ruleset generated
        # from an allow-list that fails its own invariants is a ruleset nobody
        # reviewed the source of.
        for e in errors:
            print("::error::" + e, file=sys.stderr)
        raise SystemExit(
            "refusing to render: " + str(len(errors)) + " allow-list invariant(s) fail; "
            "fix infra/egress/allowlist.yaml first")
    return entries


def _addr(value, what):
    if not value:
        return ""
    try:
        addr = ipaddress.ip_address(value)
    except ValueError:
        raise SystemExit(what + " must be a single IPv4 host address, got " + repr(value))
    if addr.version != 4:
        raise SystemExit(what + " must be IPv4: this allow list is IPv4-only by decision (N-08)")
    return str(addr)


def self_check():
    """Assert the properties that only bite on a day nobody is watching.

    Every one of these is invisible in the file this repo renders today, because
    `pinned_ip` is `unset` and the interesting half of the renderer never runs.
    The first real node fills that field in, and the ordering rules below are
    what stop it from opening more than it meant to.
    """
    pinned = [{"name": "model_gateway", "tier": "sandbox", "fqdn": "gw.internal",
               "pinned_ip": "10.9.9.9", "port": 4000, "protocol": "tcp"}]
    unset = [{"name": "model_gateway", "tier": "sandbox", "fqdn": "gw.internal",
              "pinned_ip": "unset", "port": 4000, "protocol": "tcp"}]
    out = []

    def case(name, ok, detail=""):
        out.append(ok)
        print("  %-58s %s%s" % (name, "ok" if ok else "FAIL", (" -- " + detail) if detail and not ok else ""))

    nft = render_nftables(pinned, "sbx0", "10.0.0.53", "10.1.1.1")
    lines = [l.strip() for l in nft.splitlines()]

    def idx(needle, start=0):
        for i in range(start, len(lines)):
            if needle in lines[i] and not lines[i].startswith("#"):
                return i
        return -1

    accept = idx("10.9.9.9 tcp dport 4000 counter accept")
    case("the pinned destination renders an accept rule", accept >= 0)
    # Ordering, and each of these is a hole if it goes the other way.
    case("east-west drop precedes the allow-list accept",
         0 <= idx("skillhub-drop-eastwest") < accept)
    case("node-loopback drop precedes the allow-list accept",
         0 <= idx("skillhub-drop-nodelo") < accept)
    case("sandboxd-port drop precedes the allow-list accept",
         0 <= idx("skillhub-drop-sandboxd") < accept)
    case("metadata drop precedes the allow-list accept",
         0 <= idx("skillhub-drop-metadata") < accept)
    # ...and this one is a hole if it goes the SAME way as the others: the
    # gateway is an RFC1918 address, so blocking the class first would drop it.
    case("the allow-list accept precedes the RFC1918 drop",
         accept >= 0 and accept < idx("skillhub-drop-rfc1918"))
    case("only the pinned resolver is accepted for DNS",
         idx("10.0.0.53 udp dport 53 counter accept") >= 0
         and idx("skillhub-drop-dns") > idx("10.0.0.53 udp dport 53 counter accept"))
    case("every rule counts what it drops (N-06)",
         all("counter" in l for l in lines if l.startswith("iifname") or l.startswith("ip saddr")))
    case("the ip6 table carries no accept rule at all (N-08)",
         "accept" not in nft.split("table ip6 skillhub {", 1)[1])

    nft_unset = render_nftables(unset, "sbx0", "10.0.0.53", "10.1.1.1")
    case("an unset pin renders no accept rule (fail-closed)",
         "dport 4000 counter accept" not in nft_unset)
    case("an unset pin is absent from the admission list, not present-and-dead",
         json.loads(render_admission(unset))["destinations"] == [])
    case("a pinned destination reaches the admission list",
         json.loads(render_admission(pinned))["destinations"][0]["port"] == 4000)

    nft_bare = render_nftables(pinned, "sbx0", "", "")
    case("no resolver configured renders no DNS accept",
         "dport 53 counter accept" not in nft_bare)
    case("no control plane configured renders no inbound accept",
         "tcp dport 9000 counter accept" not in nft_bare)

    dns = render_dnsmasq(pinned, "10.0.0.53")
    case("the resolver answers the pin and NXDOMAINs the rest (T5-1)",
         "address=/gw.internal/10.9.9.9" in dns and "address=/#/" in dns)
    case("the resolver never answers AAAA (N-08)", "filter-AAAA" in dns)
    dns_unset = render_dnsmasq(unset, "10.0.0.53")
    case("an unset pin leaves its name NXDOMAIN too",
         "address=/gw.internal/" not in dns_unset)

    print("self-check: %d/%d" % (sum(out), len(out)))
    return 0 if all(out) else 1


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("--out", type=pathlib.Path, default=DEFAULT_OUT)
    p.add_argument("--check", action="store_true",
                   help="render and compare against --out instead of writing; non-zero on drift")
    p.add_argument("--self-check", action="store_true",
                   help="assert the renderer's own invariants; touches no file")
    p.add_argument("--sandbox-iface", default="skillhub-sbx")
    p.add_argument("--resolver", default="",
                   help="the node's pinned DNS resolver; empty renders no DNS accept (fail-closed)")
    p.add_argument("--control-plane", default="",
                   help="source allowed to reach :9000; empty renders no inbound accept (fail-closed)")
    args = p.parse_args(argv)

    if args.self_check:
        return self_check()

    entries = load_entries(ALLOWLIST)
    files = render_all(entries, args.sandbox_iface,
                       _addr(args.resolver, "--resolver"),
                       _addr(args.control_plane, "--control-plane"))

    if args.check:
        drift = []
        for name, body in files.items():
            path = args.out / name
            if not path.exists():
                drift.append(str(path) + " is missing")
            elif path.read_text(encoding="utf-8") != body:
                drift.append(str(path) + " differs from what allowlist.yaml renders")
        for d in drift:
            print("::error::" + d, file=sys.stderr)
        if drift:
            print("run: python3 tools/egress/render.py --out " + str(args.out), file=sys.stderr)
            return 1
        print("egress render: " + str(len(files)) + " file(s) match infra/egress/allowlist.yaml")
        return 0

    args.out.mkdir(parents=True, exist_ok=True)
    for name, body in files.items():
        with open(args.out / name, "w", encoding="utf-8", newline="\n") as fh:
            fh.write(body)
    dests = rendered_destinations(entries)
    print("egress render: " + str(len(files)) + " file(s) into " + str(args.out)
          + "; " + str(len(dests)) + " reachable destination(s)")
    if not dests:
        print("::warning::no destination has a pinned address, so this node reaches nothing (fail-closed)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
