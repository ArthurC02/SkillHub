#!/usr/bin/env bash
# ADR-022 part three, T5 — the network exfiltration items, run against the real
# rendered ruleset in a real kernel.
#
# ============================ WHAT THIS IS NOT ==============================
# This is NOT the SEC-009 acceptance, and it is not Suite 2. ADR-022 puts T5 in
# Suite 2 and defines Suite 2's subject as *the node that is about to join the
# pool*, built by production IaC, with the production nftables and dnsmasq
# already applied. This script builds a topology that imitates that node inside
# one privileged container, so what it measures is the ruleset, not the node.
#
# The distinction is not a formality. Three things here are lab-shaped:
#   - the kernel is Docker Desktop's WSL2 VM, not the node's
#   - the "sandboxes" are network namespaces, not gVisor sandboxes
#   - the destinations are namespaces on a fake uplink, not the real gateway
# A pass says the rules do what they say to traffic shaped like a sandbox's. It
# says nothing about whether the node that eventually runs them is configured
# the way this lab configures itself.
#
# What it IS for, and why it was worth writing before there is a node:
#   1. Until 2026-08-27 the rendered ruleset had never been loaded by any
#      kernel. `render.py --check` proves the file matches the allow-list; it
#      cannot prove nft would accept the file, and a ruleset that fails to load
#      leaves a node with NO rules at all — the direction that fails open.
#   2. T5 had no executable form. The deployment batch would have written it
#      under time pressure, on a machine nobody had debugged, against a node
#      that was already being provisioned.
#   3. Every drop here is graded by the counter on the rule that was supposed to
#      make it, not by "the connection failed". A missing route fails a
#      connection too, and a test that cannot tell those apart passes on a node
#      with no network at all.
#
# Usage: tools/sec009/t5-network-egress.sh
#   SEC009_NO_NFT=1   negative control: build the same topology, load no rules,
#                     and require every attempt to SUCCEED. Probes that cannot
#                     see a failure are measuring nothing.
#   SEC009_T5_DROP_RULE=<name>
#                     mutation: strip one drop rule by its log prefix before
#                     loading, and expect the probe that depends on it to turn
#                     FAIL. A grader that has never been red grades nothing.
# Exit:  0 every attempt refused with the expected rule counting it, and the
#          pinned destination reachable
#        1 setup failed — nothing was measured, which is not a pass
#        2 something was not refused, was refused by the wrong rule, or could
#          not be checked
#
# Vocabulary is ADR-022's, including `unknown` counting as fail: "we could not
# look" must never read the same as "we looked and it was fine".
set -uo pipefail

IMAGE="${T5_IMAGE:-debian:12-slim}"
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

docker run --rm -i --privileged \
  -e "SEC009_NO_NFT=${SEC009_NO_NFT:-0}" \
  -e "SEC009_T5_DROP_RULE=${SEC009_T5_DROP_RULE:-}" \
  -v "${REPO}/tools:/src/tools:ro" \
  -v "${REPO}/infra:/src/infra:ro" \
  "$IMAGE" bash -s <<'INNER'
set -uo pipefail
NO_NFT="${SEC009_NO_NFT:-0}"
fail=0
reached=0

apt-get -qq update >/dev/null 2>&1
apt-get -qq install -y nftables iproute2 iputils-ping netcat-openbsd \
  dnsmasq bind9-dnsutils python3 python3-yaml >/dev/null 2>&1 || {
    echo "setup: package install failed" >&2; exit 1; }

# ---------------------------------------------------------------- the ruleset
#
# Rendered here rather than read from infra/egress/rendered/, and from a
# *modified* allow-list, because the committed one pins nothing: every
# destination is `unset`, so the ruleset it renders has no accept rule and a
# sandbox on such a node reaches nothing at all. That is the right production
# default and it makes the whole of T5 pass for the wrong reason — ADR-022's own
# 反向驗證 says a run that reaches nothing is not evidence that the rules work.
#
# So the lab pins the gateway to an address it actually stands up. The renderer
# under test is the real one, byte for byte; only its input differs, and the
# committed input is loaded too (see "the committed ruleset loads" below).
mkdir -p /work && cp -r /src/tools /src/infra /work/
sed -i 's/pinned_ip: unset/pinned_ip: 198.51.100.10/' /work/infra/egress/allowlist.yaml
grep -q 'pinned_ip: 198.51.100.10' /work/infra/egress/allowlist.yaml || {
    echo "setup: could not pin the lab gateway address" >&2; exit 1; }

python3 /work/tools/egress/render.py --out /work/rendered \
  --resolver 10.77.0.1 --control-plane 198.51.100.30 || {
    echo "setup: render.py failed" >&2; exit 1; }

# The committed ruleset — the one a node would get today — has to load too. It
# renders zero accept rules, so it is never the ruleset under test below, but
# "nft accepts this file" is exactly the property that was never checked.
if nft -f /src/infra/egress/rendered/nftables.conf; then
  echo "  committed ruleset loads                 PASS    (0 accept rules, fail-closed)"
else
  echo "  committed ruleset loads                 FAIL    nft refused the committed file"
  fail=2
fi
nft flush ruleset

# ---------------------------------------------------------------- the topology
#
#   run1, run2 ── skillhub-sbx (bridge, 10.77.0.1/24) ── node ── skillhub-up ── up
#                                                                               │
#   up holds every destination T5 names: the pinned gateway, a provider stand-in
#   for N-07, the metadata address, an RFC1918 host, a resolver, and the source
#   address the inbound rule is supposed to admit.
echo 1 > /proc/sys/net/ipv4/ip_forward
echo 1 > /proc/sys/net/ipv6/conf/all/forwarding

# T5-4 depends on this sysctl and nothing in the repo says so. Two sandboxes on
# one bridge in one subnet talk over layer 2; the `ip` forward hook never sees
# those frames unless bridged IPv4 traffic is passed through it. The rule reads
# like it protects east-west on its own. It does not.
BRNF=/proc/sys/net/bridge/bridge-nf-call-iptables
if [ -f "$BRNF" ]; then
  echo 1 > "$BRNF"
  BRNF_STATE="$(cat "$BRNF")"
else
  BRNF_STATE="absent"
fi

ip link add skillhub-sbx type bridge
ip addr add 10.77.0.1/24 dev skillhub-sbx
ip addr add fd00:77::1/64 dev skillhub-sbx nodad
ip link set skillhub-sbx up

mkrun() {
  ip netns add "$1"
  ip link add "v-$1" type veth peer name eth0 netns "$1"
  ip link set "v-$1" master skillhub-sbx up
  ip -n "$1" addr add "$2/24" dev eth0
  ip -n "$1" addr add "$3/64" dev eth0 nodad
  ip -n "$1" link set eth0 up
  ip -n "$1" link set lo up
  ip -n "$1" route add default via 10.77.0.1
  ip -n "$1" -6 route add default via fd00:77::1
}
mkrun run1 10.77.0.11 fd00:77::11
mkrun run2 10.77.0.12 fd00:77::12

ip netns add up
ip link add skillhub-up type veth peer name eth0 netns up
ip addr add 198.51.100.1/24 dev skillhub-up
ip addr add fd00:51::1/64 dev skillhub-up nodad
ip link set skillhub-up up
ip -n up link set lo up
ip -n up link set eth0 up
for a in 198.51.100.10/24 198.51.100.20/32 198.51.100.30/32 198.51.100.53/32 \
         169.254.169.254/32 10.99.0.5/32; do
  ip -n up addr add "$a" dev eth0
done
ip -n up addr add fd00:51::10/64 dev eth0 nodad
ip -n up route add default via 198.51.100.1
ip -n up -6 route add default via fd00:51::1
ip route add 169.254.0.0/16 via 198.51.100.10 dev skillhub-up
ip route add 10.99.0.0/24 via 198.51.100.10 dev skillhub-up

# Listeners. Without them a refusal cannot be told from an empty address, which
# is the same mistake as grading a drop by "the connection failed".
LISTEN='
import socket, sys, threading
for spec in sys.argv[1:]:
    host, port = spec.rsplit(":", 1)
    s = socket.socket(socket.AF_INET6 if ":" in host else socket.AF_INET, socket.SOCK_STREAM)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind((host, int(port)))
    s.listen(16)
    threading.Thread(target=lambda s=s: [s.accept() for _ in iter(int, 1)], daemon=True).start()
threading.Event().wait()
'
ip netns exec up python3 -c "$LISTEN" \
  198.51.100.10:4000 198.51.100.10:5432 198.51.100.10:9000 198.51.100.10:8080 \
  198.51.100.20:443 198.51.100.53:53 169.254.169.254:80 10.99.0.5:22 fd00:51::10:4000 &
# The node's own two: the sandboxd port a run must not reach, and the same port
# on the uplink address, which is what T5-8 probes from outside.
python3 -c "$LISTEN" 127.0.0.1:9000 10.77.0.1:9000 198.51.100.1:9000 &
# The east-west target. Without it, T5-4 grades a closed port as a policy drop.
ip netns exec run2 python3 -c "$LISTEN" 10.77.0.12:22 &
sleep 1

# The node's own resolver, from the rendered dnsmasq config. T5-1 has two
# halves and this is the one that needs a resolver to exist: a name outside the
# allow-list must come back NXDOMAIN rather than be forwarded somewhere the ip
# table cannot see.
ip addr add 10.77.0.1/32 dev lo 2>/dev/null
dnsmasq --conf-file=/work/rendered/dnsmasq.conf --pid-file=/run/dnsmasq.pid 2>/dev/null
DNSMASQ_UP=$?

if [ "$NO_NFT" = 1 ]; then
  echo
  echo 'T5 network egress (NEGATIVE CONTROL: no ruleset loaded, everything should be reachable):'
else
  # A mutation hook, because a harness that has never been red is not evidence
  # that it can go red. SEC009_T5_DROP_RULE=metadata strips the rule with that
  # log prefix before loading; the matching probe must turn FAIL. This is the
  # AGENTS.md rule 9 check for a script whose whole job is grading other things.
  if [ -n "${SEC009_T5_DROP_RULE:-}" ]; then
    grep -v "skillhub-drop-${SEC009_T5_DROP_RULE} " /work/rendered/nftables.conf > /work/mutated.conf
    echo "  MUTATION: rule skillhub-drop-${SEC009_T5_DROP_RULE} removed before loading"
    nft -f /work/mutated.conf || { echo "setup: mutated ruleset failed to load" >&2; exit 1; }
  else
    nft -f /work/rendered/nftables.conf || { echo "setup: lab ruleset failed to load" >&2; exit 1; }
  fi
  echo
  echo 'T5 network egress (rendered ruleset loaded):'
fi

# ---------------------------------------------------------------- grading
#
# A drop is graded by the counter on the rule that was supposed to make it. The
# alternative — "the connection failed" — passes identically on a node with a
# broken route, which is how a network test ends up asserting nothing.
counter() {
  [ "$NO_NFT" = 1 ] && { echo 0; return; }
  local pat="skillhub-drop-$1 "
  [ "$1" = "*" ] && pat="counter packets"
  nft list ruleset 2>/dev/null \
    | grep -F "$pat" \
    | sed -n 's/.*counter packets \([0-9]*\).*/\1/p' \
    | awk '{t += $1} END {print t + 0}'
}

# probe <label> <expected-drop-rule> <command...>
#
# The rule name is what has to count the drop. Two names are special:
#   -       refused, do not look for a counter
#   kernel  refused BEFORE nftables saw the packet, and the assertion is that
#           NO counter moved. That is a claim about where the refusal comes
#           from, and it is the only way to write down "this one is structural"
#           without it reading like a rule that works.
probe() {
  local name="$1" rule="$2"; shift 2
  local before after
  before=$(counter "$rule")
  [ "$rule" = "kernel" ] && before=$(counter '*')
  if "$@" >/dev/null 2>&1; then
    if [ "$NO_NFT" = 1 ]; then
      printf '  %-38s PASS    reached, as an unruled node should\n' "$name"
      reached=$((reached + 1))
    else
      printf '  %-38s FAIL    the attempt succeeded\n' "$name"
      fail=2
    fi
    return
  fi
  if [ "$NO_NFT" = 1 ]; then
    if [ "$rule" = "kernel" ]; then
      # Exempt, and the exemption is the finding: this destination is
      # unreachable with or without a ruleset, so no negative control can make
      # it reachable. See the grading note above.
      printf '  %-38s PASS    unreachable with no rules either — refused by the stack, not by policy\n' "$name"
      return
    fi
    printf '  %-38s FAIL    unreachable even with no rules — this probe measures nothing\n' "$name"
    fail=2
    return
  fi
  after=$(counter "$rule")
  if [ "$rule" = "-" ]; then
    printf '  %-38s PASS    refused\n' "$name"
  elif [ "$rule" = "kernel" ]; then
    after=$(counter '*')
    if [ "$after" -gt "$before" ]; then
      printf '  %-38s FAIL    something in nftables counted it after all (%s -> %s)\n' "$name" "$before" "$after"
      fail=2
    else
      printf '  %-38s PASS    refused, and no nftables rule saw it (measured)\n' "$name"
    fi
  elif [ "$after" -gt "$before" ]; then
    printf '  %-38s PASS    refused, %s counted it (%s -> %s)\n' "$name" "$rule" "$before" "$after"
  else
    # N-06: blocked with no record is a fail even though the traffic did not
    # leave. ADR-022's T5 criterion says so in as many words.
    printf '  %-38s FAIL    refused, but skillhub-drop-%s counted nothing\n' "$name" "$rule"
    fail=2
  fi
}

R1="ip netns exec run1"
TCP="nc -z -w2"

probe 'T5-1 DNS to an outside resolver' dns \
  $R1 $TCP 198.51.100.53 53
# ADR-022 says 1000 ports; this is one, because every port costs the full drop
# timeout and the item's question is whether the range is reachable at all, not
# how many closed ports it has. The deviation is stated rather than averaged
# away: a node run must do the full sweep.
probe 'T5-2 internal scan (RFC1918)' rfc1918 \
  $R1 $TCP 10.99.0.5 22
probe 'T5-3 metadata service' metadata \
  $R1 $TCP 169.254.169.254 80
probe 'T5-4 east-west to another run' eastwest \
  $R1 $TCP 10.77.0.12 22
# T5-5's three halves are refused in three different places, and the lab run of
# 2026-08-27 is what established that. Grading each against the rule that reads
# like it does the work would have passed two of them for the wrong reason.
#
#   node loopback      never leaves the sandbox's namespace. 127.0.0.0/8 routes
#                      to that namespace's own lo, and the kernel will not send
#                      a 127/8 destination out a non-loopback device even with
#                      a route added and route_localnet=1 on both sides — an
#                      explicit prerouting probe counted zero packets. So the
#                      forward chain's `ip daddr 127.0.0.0/8` rule cannot fire
#                      for this direction: a loopback destination is never
#                      forwarded. The rule is not wrong, it is unreachable, and
#                      what refuses this attempt is the network stack.
#   bridge gateway     the node's own address, so the packet is delivered
#                      locally: input hook, not forward. The forward chain's
#                      `tcp dport 9000` rule never sees it; `policy drop` in the
#                      input chain does, and counts it as skillhub-drop-inbound.
#   gateway:9000       forwarded traffic to another host, so the forward rule
#                      does fire — see T5-7 below.
probe 'T5-5 node loopback' kernel \
  $R1 $TCP 127.0.0.1 9000
probe 'T5-5 sandboxd port on the bridge' inbound \
  $R1 $TCP 10.77.0.1 9000
probe 'T5-6 provider domain direct (N-07)' default \
  $R1 $TCP 198.51.100.20 443
probe 'T5-7 gateway address, Postgres port' default \
  $R1 $TCP 198.51.100.10 5432
probe 'T5-7 gateway address, sandboxd port' sandboxd \
  $R1 $TCP 198.51.100.10 9000
probe 'T5-7 gateway address, platform API' default \
  $R1 $TCP 198.51.100.10 8080
probe 'T5-9 IPv6 bypass to the gateway' v6 \
  $R1 $TCP -6 fd00:51::10 4000

# T5-8 is inbound and therefore not a run's attempt: it is the node's input
# chain, probed from outside. The control-plane source is rendered as an accept,
# so both halves are here — a rule that admits nobody would pass the drop half
# and mean nothing.
probe 'T5-8 inbound to :9000, other source' inbound \
  ip netns exec up nc -z -w2 -s 198.51.100.10 198.51.100.1 9000

echo
if [ "$NO_NFT" != 1 ]; then
  # 反向驗證 (ADR-022 T5): the pinned destination must be reachable, or every
  # line above is "this node has no network" wearing a security result.
  if $R1 $TCP 198.51.100.10 4000 >/dev/null 2>&1; then
    printf '  %-38s PASS    the pinned IP:port is reachable\n' 'reverse check: gateway reachable'
  else
    printf '  %-38s FAIL    nothing is reachable, so the drops above prove nothing\n' 'reverse check: gateway reachable'
    fail=2
  fi

  # T5-8's other half. An input chain that admits nobody passes the drop probe
  # above and means nothing; the rendered rule names one control-plane source
  # and that source has to get through.
  if ip netns exec up nc -z -w2 -s 198.51.100.30 198.51.100.1 9000 >/dev/null 2>&1; then
    printf '  %-38s PASS    the rendered control-plane source is admitted\n' 'reverse check: :9000 from control plane'
  else
    printf '  %-38s FAIL    nobody can reach :9000, so the drop above proves nothing\n' 'reverse check: :9000 from control plane'
    fail=2
  fi

  # T5-1's other half. dnsmasq answers, and the answer for a name outside the
  # allow-list is NXDOMAIN — not a forward to somewhere the ip table cannot see.
  if [ "${DNSMASQ_UP:-1}" -eq 0 ]; then
    ans=$(dig +time=2 +tries=1 @10.77.0.1 exfil.example.com TXT 2>/dev/null | sed -n 's/^;; ->>HEADER<<-.*status: \([A-Z]*\).*/\1/p')
    if [ "$ans" = "NXDOMAIN" ]; then
      printf '  %-38s PASS    %s\n' 'T5-1 resolver answers NXDOMAIN' "$ans"
    else
      printf '  %-38s FAIL    status=%s, want NXDOMAIN\n' 'T5-1 resolver answers NXDOMAIN' "${ans:-no answer}"
      fail=2
    fi
  else
    printf '  %-38s UNKNOWN dnsmasq did not start\n' 'T5-1 resolver answers NXDOMAIN'
    [ $fail -eq 0 ] && fail=2
  fi
fi

echo
echo "node kernel:              $(uname -sr)"
echo "nft:                      $(nft --version)"
echo "bridge-nf-call-iptables:  ${BRNF_STATE}"

if [ "$NO_NFT" = 1 ]; then
  if [ "$reached" -gt 0 ] && [ $fail -eq 0 ]; then
    echo "NEGATIVE CONTROL OK: ${reached} attempt(s) reached their destination unruled, so these probes can see a failure"
    exit 0
  fi
  echo 'NEGATIVE CONTROL FAILED: the probes did not reach their destinations even with no rules'
  exit 2
fi
if [ $fail -eq 0 ]; then
  echo 'T5: every attempt refused by the rule meant to refuse it; the pinned destination reachable'
else
  echo 'T5: something was not refused, was refused by the wrong rule, or could not be checked'
fi
exit $fail
INNER
rc=$?
case $rc in
  0) echo "t5 network egress: procedure executed, all refused — NOT the SEC-009 acceptance" ;;
  1) echo "t5 network egress: setup failed, nothing was measured" >&2 ;;
  *) echo "t5 network egress: FAILURES above" >&2 ;;
esac
exit $rc
