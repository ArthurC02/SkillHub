#!/usr/bin/env python3
"""Assert ADR-022 Q3's invariants on infra/egress/allowlist.yaml.

Run it anywhere: `python3 .github/workflows/check_egress_allowlist.py`.
Exit 0 = the allow-list still matches the decision; exit 1 = it does not, and the
message says which part of ADR-022 to go read.
"""
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

errors, warnings = [], []
entries = yaml.safe_load(PATH.read_text(encoding="utf-8"))["destinations"] or []

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

# Node tier talks to third parties whose IPs rotate; a pin there is a rule that
# breaks silently on the vendor's next deploy.
for e in node:
    if e.get("pinned_ip"):
        warnings.append(f"{e.get('name')}: tier:node must not pin an IP (ADR-022 A1-a)")
    if str(e.get("fqdn", "")).endswith(".internal"):
        warnings.append(f"{e.get('name')}: platform-owned host on tier:node — should it be tier:sandbox?")

for w in warnings:
    print(f"::warning::{w}")
for err in errors:
    print(f"::error::{err}")
print(f"checked {len(entries)} destinations: {len(sandbox)} sandbox, {len(node)} node")
sys.exit(1 if errors else 0)
