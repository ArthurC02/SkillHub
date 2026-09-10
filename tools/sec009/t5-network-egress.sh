#!/usr/bin/env bash
# Loads the real rendered nftables/dnsmasq egress rules into a lab topology
# (network namespaces standing in for sandboxes, on a fake uplink) and probes
# each one, grading a drop by the rule counter that fired, not by "it failed".
set -uo pipefail

IMAGE="${T5_IMAGE:-debian:12-slim}"
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# /lib/modules is mounted read-only because modprobe inside a container reads
# the container's own module tree, not the host's.
docker run --rm -i --privileged \
  -e "SEC009_NO_NFT=${SEC009_NO_NFT:-0}" \
  -e "SEC009_T5_DROP_RULE=${SEC009_T5_DROP_RULE:-}" \
  -v "/lib/modules:/lib/modules:ro" \
  -v "${REPO}/tools:/src/tools:ro" \
  -v "${REPO}/infra:/src/infra:ro" \
  "$IMAGE" bash -s <<'INNER'
set -uo pipefail
NO_NFT="${SEC009_NO_NFT:-0}"
fail=0
reached=0

apt-get -qq update >/dev/null 2>&1
apt-get -qq install -y nftables iproute2 iputils-ping netcat-openbsd \
  dnsmasq bind9-dnsutils python3 python3-yaml kmod >/dev/null 2>&1 || {
    echo "setup: package install failed" >&2; exit 1; }

# Rendered from a copy of the allow-list with the gateway pinned to a real lab
# address: the committed allow-list pins nothing, so its ruleset has no accept
# rule and would make every probe below pass by reaching nothing.
mkdir -p /work && cp -r /src/tools /src/infra /work/
sed -i 's/pinned_ip: unset/pinned_ip: 198.51.100.10/' /work/infra/egress/allowlist.yaml
grep -q 'pinned_ip: 198.51.100.10' /work/infra/egress/allowlist.yaml || {
    echo "setup: could not pin the lab gateway address" >&2; exit 1; }

python3 /work/tools/egress/render.py --out /work/rendered \
  --resolver 10.77.0.1 --control-plane 198.51.100.30 || {
    echo "setup: render.py failed" >&2; exit 1; }

# The committed (fail-closed, zero-accept-rule) ruleset must also load cleanly,
# even though it is never the ruleset probed below.
if nft -f /src/infra/egress/rendered/nftables.conf; then
  echo "  committed ruleset loads                 PASS    (0 accept rules, fail-closed)"
else
  echo "  committed ruleset loads                 FAIL    nft refused the committed file"
  fail=2
fi
nft flush ruleset

# Topology: run1, run2 on a bridge (skillhub-sbx) behind the node; every
# destination probed below (gateway, a provider stand-in, metadata, RFC1918,
# the resolver) lives in the "up" namespace on the other side.
echo 1 > /proc/sys/net/ipv4/ip_forward
echo 1 > /proc/sys/net/ipv6/conf/all/forwarding

# The east-west drop rule depends on br_netfilter: two namespaces on one
# bridge talk over layer 2, invisible to the ip forward hook without it.
BRNF=/proc/sys/net/bridge/bridge-nf-call-iptables
BRNF_BY_LAB=no
if [ ! -f "$BRNF" ]; then
  modprobe br_netfilter 2>/dev/null && BRNF_BY_LAB=yes
fi
if [ -f "$BRNF" ]; then
  [ "$(cat "$BRNF")" = 1 ] || BRNF_BY_LAB=yes
  echo 1 > "$BRNF"
  BRNF_STATE="$(cat "$BRNF")  (loaded or enabled by the lab: ${BRNF_BY_LAB})"
else
  BRNF_STATE="absent - br_netfilter would not load, so east-west is NOT filtered here"
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
python3 -c "$LISTEN" 127.0.0.1:9000 10.77.0.1:9000 198.51.100.1:9000 &
ip netns exec run2 python3 -c "$LISTEN" 10.77.0.12:22 &
sleep 1

ip addr add 10.77.0.1/32 dev lo 2>/dev/null
dnsmasq --conf-file=/work/rendered/dnsmasq.conf --pid-file=/run/dnsmasq.pid 2>/dev/null
DNSMASQ_UP=$?

if [ "$NO_NFT" = 1 ]; then
  echo
  echo 'T5 network egress (NEGATIVE CONTROL: no ruleset loaded, everything should be reachable):'
else
  # SEC009_T5_DROP_RULE=<name> strips the rule with that log prefix before
  # loading, so the matching probe can be checked to actually turn FAIL.
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
# Rule name "-" means refused with no counter to check; "kernel" means refused
# before nftables ever saw the packet, asserted as no counter moving at all.
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
    printf '  %-38s FAIL    refused, but skillhub-drop-%s counted nothing\n' "$name" "$rule"
    fail=2
  fi
}

R1="ip netns exec run1"
TCP="nc -z -w2"

probe 'T5-1 DNS to an outside resolver' dns \
  $R1 $TCP 198.51.100.53 53
probe 'T5-2 internal scan (RFC1918)' rfc1918 \
  $R1 $TCP 10.99.0.5 22
probe 'T5-3 metadata service' metadata \
  $R1 $TCP 169.254.169.254 80
probe 'T5-4 east-west to another run' eastwest \
  $R1 $TCP 10.77.0.12 22
# Refused in three different places: loopback never forwards at all, the
# bridge gateway is delivered locally, only a real forward hits the rule.
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

probe 'T5-8 inbound to :9000, other source' inbound \
  ip netns exec up nc -z -w2 -s 198.51.100.10 198.51.100.1 9000

echo
if [ "$NO_NFT" != 1 ]; then
  # The pinned destination must be reachable, or every drop above is "this
  # node has no network" wearing a security result.
  if $R1 $TCP 198.51.100.10 4000 >/dev/null 2>&1; then
    printf '  %-38s PASS    the pinned IP:port is reachable\n' 'reverse check: gateway reachable'
  else
    printf '  %-38s FAIL    nothing is reachable, so the drops above prove nothing\n' 'reverse check: gateway reachable'
    fail=2
  fi

  if ip netns exec up nc -z -w2 -s 198.51.100.30 198.51.100.1 9000 >/dev/null 2>&1; then
    printf '  %-38s PASS    the rendered control-plane source is admitted\n' 'reverse check: :9000 from control plane'
  else
    printf '  %-38s FAIL    nobody can reach :9000, so the drop above proves nothing\n' 'reverse check: :9000 from control plane'
    fail=2
  fi

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
echo "NODE REQUIREMENT (T5-4):  br_netfilter loaded and bridge-nf-call-iptables=1."
echo "                          Without it that rule counts zero forever and two"
echo "                          runs on one node reach each other."

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
