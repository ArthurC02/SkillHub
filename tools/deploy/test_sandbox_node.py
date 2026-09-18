#!/usr/bin/env python3
import json
import os
import pathlib
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
BIN = ROOT / "infra" / "deploy" / "sandbox" / "bin"
DIGEST = "@sha256:" + "a" * 64
TOKEN = "t" * 32


def fake(directory, name, body):
    path = directory / name
    path.write_text("#!/bin/sh\n" + body + "\n")
    path.chmod(0o755)


class Node:
    def __init__(self):
        self.root = pathlib.Path(tempfile.mkdtemp())
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.secrets = self.root / "secrets"
        self.secrets.mkdir()
        self.allow = self.root / "egress-allow.json"
        self.allow.write_text('{"destinations": []}\n')
        self.secret(TOKEN)
        fake(self.bin, "nft", '[ "$*" = "list table ip skillhub" ]')
        fake(self.bin, "sysctl", 'case "$2" in net.bridge.*) echo "$FAKE_BRIDGE_NF" ;; *) echo "$FAKE_CONNTRACK_ACCT" ;; esac')
        fake(self.bin, "docker", 'echo "$FAKE_RUNTIMES"')
        self.env = {
            "PATH": "%s:%s" % (self.bin, os.environ["PATH"]),
            "SKILLHUB_SECRETS_DIR": str(self.secrets),
            "SKILLHUB_SANDBOX_EGRESS_ALLOW": str(self.allow),
            "SKILLHUB_SANDBOXD_IMAGE": "ghcr.io/o/skillhub-sandboxd:abc" + DIGEST,
            "SKILLHUB_SANDBOX_IMAGE": "ghcr.io/o/skillhub-runtime-agent-sdk:2026.08-12" + DIGEST,
            "FAKE_BRIDGE_NF": "1",
            "FAKE_CONNTRACK_ACCT": "1",
            "FAKE_RUNTIMES": "io.containerd.runc.v2 runc runsc ",
        }

    def secret(self, token, mode=0o600):
        path = self.secrets / "sandboxd.env"
        path.write_text("SKILLHUB_SANDBOX_TOKEN=%s\n" % token)
        path.chmod(mode)

    def preflight(self, **env):
        result = subprocess.run(["sh", str(BIN / "skillhub-preflight")], env=dict(self.env, **env),
                                capture_output=True, text=True)
        return result.returncode, result.stderr


def refuses(fragment, node, **env):
    status, stderr = node.preflight(**env)
    assert status == 1 and fragment in stderr, (status, stderr)


def test_a_node_meeting_every_condition_passes_preflight_silently():
    assert Node().preflight() == (0, "")


def test_a_token_of_32_characters_passes_and_31_is_refused():
    node = Node()
    node.secret("t" * 31)
    refuses("does not set SKILLHUB_SANDBOX_TOKEN to at least 32 characters", node)
    node.secret("t" * 32)
    assert node.preflight() == (0, "")


def test_a_secret_readable_by_others_is_refused():
    node = Node()
    node.secret(TOKEN, mode=0o644)
    refuses("sandboxd.env is mode 644; secrets must be 600", node)


def test_a_missing_secret_is_refused():
    node = Node()
    (node.secrets / "sandboxd.env").unlink()
    refuses("sandboxd.env is missing or empty", node)


def test_either_image_pinned_only_by_tag_is_refused():
    for name in ("SKILLHUB_SANDBOXD_IMAGE", "SKILLHUB_SANDBOX_IMAGE"):
        refuses("%s='ghcr.io/o/x:1' is not pinned by digest" % name, Node(), **{name: "ghcr.io/o/x:1"})


def test_a_missing_egress_admission_list_is_refused():
    node = Node()
    node.allow.unlink()
    refuses("egress-allow.json is missing", node)


def test_an_unloaded_ruleset_is_refused():
    node = Node()
    fake(node.bin, "nft", "exit 1")
    refuses("the skillhub nftables table is not loaded", node)


def test_bridge_traffic_bypassing_netfilter_is_refused():
    refuses("net.bridge.bridge-nf-call-iptables is not 1", Node(), FAKE_BRIDGE_NF="0")


def test_flow_records_without_byte_counts_are_refused():
    refuses("nf_conntrack_acct is not 1", Node(), FAKE_CONNTRACK_ACCT="0")


def test_a_docker_without_runsc_is_refused():
    refuses("dockerd has no runsc runtime registered", Node(), FAKE_RUNTIMES="io.containerd.runc.v2 runc ")


def test_every_failed_condition_is_reported_not_only_the_first():
    status, stderr = Node().preflight(FAKE_BRIDGE_NF="0", FAKE_RUNTIMES="runc ")
    assert status == 1 and "bridge-nf-call-iptables" in stderr and "runsc" in stderr, stderr


def mark_serving(isolation):
    node = Node()
    facts = node.root / "node.json"
    facts.write_text(json.dumps({"node_id": "n1", "role": "sandbox-exec", "build_phase": "provision"}) + "\n")
    config = node.root / "sandboxd.env"
    config.write_text("SKILLHUB_SANDBOX_RUNTIME=runsc\nSKILLHUB_SANDBOX_ADDR=10.0.0.4:9000\n")
    capability = json.dumps({"isolation": {"strength": isolation}})
    fake(node.bin, "curl", 'cat >"$FAKE_CURL_CONFIG"; echo "$*" >"$FAKE_CURL_ARGS"; printf %%s \'%s\'' % capability)
    fake(node.bin, "sleep", "exit 0")
    env = dict(node.env, SKILLHUB_NODE_FACTS=str(facts), SKILLHUB_SANDBOX_CONFIG=str(config),
               FAKE_CURL_CONFIG=str(node.root / "curl-config"), FAKE_CURL_ARGS=str(node.root / "curl-args"))
    result = subprocess.run(["sh", str(BIN / "skillhub-mark-serving")], env=env, capture_output=True, text=True)
    return result, json.loads(facts.read_text()), node.root


def test_a_node_isolating_strongly_is_marked_serving_and_the_token_never_reaches_argv():
    result, facts, root = mark_serving("strong")
    assert result.returncode == 0, result.stderr
    assert facts == {"node_id": "n1", "role": "sandbox-exec", "build_phase": "serving"}, facts
    assert (root / "curl-config").read_text() == 'header = "Authorization: Bearer %s"\n' % TOKEN
    args = (root / "curl-args").read_text()
    assert TOKEN not in args and "http://10.0.0.4:9000/capability" in args, args


def test_a_node_that_isolates_weakly_stays_in_provision():
    result, facts, _ = mark_serving("weak")
    assert result.returncode == 1 and "isolation strength 'weak', not strong" in result.stderr, result.stderr
    assert facts["build_phase"] == "provision", facts


if __name__ == "__main__":
    failures = 0
    for name, test in sorted(globals().items()):
        if name.startswith("test_") and callable(test):
            try:
                test()
                print("ok   %s" % name)
            except AssertionError as error:
                failures += 1
                print("FAIL %s: %s" % (name, error))
    sys.exit(1 if failures else 0)
