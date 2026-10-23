from __future__ import annotations

import functools
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any, ClassVar
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))

from domain_registry.audit import append_locked
from domain_registry.changes import (
    init_change_package,
    validate_change_package,
)
from domain_registry.common import ASSET_KEYS, registry_dir, writer_lock
from domain_registry.evidence import (
    citation,
    verify,
)
from domain_registry.hitl import (
    finalize_proposal,
    record_approval,
    submit_proposal,
    verify_proposal,
)
from domain_registry.git_hooks import install_pre_push_hook
from domain_registry.impact import boundary_impact
from domain_registry.policy import amend_policy_fields
from domain_registry.registry import (
    init_registry,
    validate,
)
from domain_registry.search import _stem, _words, ranked_lookup
from domain_registry.signing import SIGNING_KEY_ENV, init_signing_key
from domain_registry.security import scan_report
from domain_registry.sources import (
    source_kind_index,
    discover_sources,
    selected_source_map,
    verify_source_map,
    write_source_map,
)
from domain_registry.store import (
    INDEX,
    read_document,
    write_document,
)
from domain_registry.transaction import transaction_path
from domain_registry.updates import (
    apply_approved_updates,
    upsert_candidate,
)


VERIFIED = [{"by": "human:Reviewer", "at": "2026-01-01T00:00:00Z"}]


def append_audit(root: Path, event: dict) -> None:
    with writer_lock(root):
        append_locked(root, event)


def applied_by(root: Path, proposal_id: str, *records: str) -> None:
    append_audit(root, {"operation": "apply-approved-updates", "proposal_id": proposal_id, "records": list(records)})


def scan_secrets(root: Path) -> list[dict]:
    return scan_report(root)["findings"]


def amend_policy(
    root: Path, field: str, value: str, reason: str, verifier: str | None = None
) -> dict:
    return amend_policy_fields(root, [(field, value)], reason, verifier)[0]


def snapshot_of_files(directory: Path) -> dict[str, bytes]:
    return {
        path.relative_to(directory).as_posix(): path.read_bytes()
        for path in sorted(directory.rglob("*"))
        if path.is_file()
    }


def lookup(root: Path, asset: str, query: str) -> list[dict[str, Any]]:
    return ranked_lookup(root, asset, query)[0]


def word_stems(text: str) -> set[str]:
    return {_stem(word) for word in _words(text)}


def source_kind_for(source_map: dict[str, Any], cited_path: str) -> str:
    return source_kind_index(source_map)(cited_path)


def stored_policy_of(repo: Path) -> dict:
    return json.loads(
        (repo / "memory" / "domain-memory-policy.json").read_text(encoding="utf-8")
    )


@functools.cache
def _seed_repository() -> tempfile.TemporaryDirectory:
    seed = tempfile.TemporaryDirectory()
    repo = Path(seed.name)
    init_registry(repo / "memory")
    (repo / "memory" / "domain-memory-policy.json").write_text(
        json.dumps(
            {
                "format": "domain-memory-policy/v1",
                "storage_mode": "tracked",
                "data_classification": "internal",
                "review_mode": "scm-verified",
                "review_governance": {
                    "verifier": "git-signed-commit",
                    "trigger": "git-push",
                    "ci_requirement": "required",
                    "authorized_signers": ["SHA256:seed"],
                },
                "source_policy": {
                    "selected_paths": ["docs"],
                    "include": ["**"],
                    "exclude": [],
                    "authority": "test",
                },
                "limits": {
                    "max_file_count": 1,
                    "max_file_bytes": 1,
                    "max_total_bytes": 1,
                    "max_query_results": 1,
                },
                "approved_command_profiles": ["unit"],
            }
        ),
        encoding="utf-8",
    )
    subprocess.run(["git", "init", "-q"], cwd=repo, check=True)
    for key, value in (("core.autocrlf", "false"), ("gc.auto", "0"), ("maintenance.auto", "false")):
        subprocess.run(["git", "config", key, value], cwd=repo, check=True)
    subprocess.run(["git", "add", "."], cwd=repo, check=True)
    subprocess.run(
        [
            "git",
            "-c",
            "user.name=Test",
            "-c",
            "user.email=test@example.com",
            "-c",
            "commit.gpgsign=false",
            "commit",
            "-qm",
            "seed",
        ],
        cwd=repo,
        check=True,
    )
    return seed


def orders_context(**fields: Any) -> dict[str, Any]:
    return {"id": "orders", "name": "Orders", "responsibility": "Own orders.", **fields}


class RegistryTestCase(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.repo = Path(self.temporary.name)
        shutil.copytree(_seed_repository().name, self.repo, dirs_exist_ok=True)

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def commit_all(self, message: str) -> None:
        subprocess.run(["git", "add", "-A"], cwd=self.repo, check=True)
        subprocess.run(
            [
                "git",
                "-c",
                "user.name=Test",
                "-c",
                "user.email=test@example.com",
                "-c",
                "commit.gpgsign=false",
                "commit",
                "-qm",
                message,
            ],
            cwd=self.repo,
            check=True,
        )

    def committed_source_map(self) -> Path:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "kept.md").write_text("kept", encoding="utf-8")
        self.commit_all("sources")
        path = self.repo / "memory" / "source-map.json"
        write_source_map(path, selected_source_map(self.repo, [self.repo / "docs"]))
        self.commit_all("source map")
        return path

    def readme_citation(self) -> dict[str, Any]:
        readme = self.repo / "README.md"
        if not readme.is_file():
            readme.write_text("evidence\n", encoding="utf-8")
        return citation(self.repo, "README.md", 1, 1)

    def write_record(self, name: str, value: Any) -> Path:
        path = self.repo / name
        path.write_text(json.dumps(value), encoding="utf-8")
        return path

    def seed(self, name: str, records: list[dict]) -> None:
        root = self.repo / "memory"
        value = read_document(root, name)
        value[ASSET_KEYS[name]] = records
        write_document(root, name, value)

    def allow_results(self, count: int) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        value = json.loads(path.read_text(encoding="utf-8"))
        value["limits"]["max_query_results"] = count
        path.write_text(json.dumps(value), encoding="utf-8")

    def two_contexts(self) -> None:
        self.seed(
            "contexts.json",
            [
                {"id": "orders", "name": "Orders", "responsibility": "Own orders."},
                {"id": "billing", "name": "Billing", "responsibility": "Own invoices."},
            ],
        )

    def add_context(self) -> None:
        record = self.write_record(
            "context.json",
            {"id": "orders", "name": "Orders", "responsibility": "Own orders."},
        )
        upsert_candidate(self.repo / "memory", self.repo, "contexts", record)

    def edit_registry(self, name: str, change) -> None:
        root = self.repo / "memory"
        value = read_document(root, name)
        change(value)
        write_document(root, name, value)

    def selected_docs(self) -> Path:
        source = self.repo / "docs"
        source.mkdir(exist_ok=True)
        (source / "rules.md").write_text("first version\n", encoding="utf-8")
        source_map_path = self.repo / "memory" / "source-map.json"
        write_source_map(source_map_path, selected_source_map(self.repo, [source]))
        self.assertEqual(
            "current", verify_source_map(self.repo, source_map_path)["status"]
        )
        return source_map_path

    def refine(self, *arguments: str) -> subprocess.CompletedProcess:
        return self.run_cli(
            "refine-sources", "--registry-root", str(self.repo / "memory"), "--repo-root", str(self.repo), *arguments
        )

    def selection_with_two_sources(self) -> None:
        self.committed_source_map()
        policy = stored_policy_of(self.repo)
        policy["limits"] = {**policy["limits"], "max_file_count": 10, "max_file_bytes": 10, "max_total_bytes": 100}
        (self.repo / "memory" / "domain-memory-policy.json").write_text(json.dumps(policy), encoding="utf-8")
        (self.repo / "fact.md").write_text("x", encoding="utf-8")
        (self.repo / "adr.md").write_text("y", encoding="utf-8")

    def sign_a_domain_memory_commit(self, message: str) -> str:
        target = self.repo / "docs" / "domain-memory"
        target.mkdir(parents=True, exist_ok=True)
        (target / "note.txt").write_text(message, encoding="utf-8")
        subprocess.run(["git", "add", "docs"], cwd=self.repo, check=True)
        subprocess.run(
            ["git", "-c", "user.name=Probe", "-c", "user.email=probe@example.com",
             "commit", "-qS", "-m", message],
            cwd=self.repo, check=True,
        )
        return self.head()

    def commit_everything(self, message: str, signed: bool = False) -> str:
        subprocess.run(["git", "add", "-A"], cwd=self.repo, check=True)
        subprocess.run(
            ["git", "-c", "user.name=Probe", "-c", "user.email=probe@example.com",
             "-c", f"commit.gpgsign={'true' if signed else 'false'}", "commit", "--allow-empty", "-qm", message],
            cwd=self.repo, check=True,
        )
        return self.head()

    def declare_check(self, package: Path, command: str) -> str:
        path = package / "test-obligations.json"
        obligations = json.loads(path.read_text(encoding="utf-8"))
        obligations["obligations"][0]["check"] = command
        path.write_text(json.dumps(obligations), encoding="utf-8")
        return command

    def run_writing(self, text: str, exit_code: int = 0) -> str:
        script = f"import sys; sys.stdout.write({text!r}); sys.exit({exit_code})"
        return f'"{sys.executable}" -c "{script}"'

    def changed_package(self, name: str, status: str, attestation: dict | None) -> None:
        package = self.repo / "changes" / name
        package.mkdir(parents=True)
        proposal = {"proposal_id": name, "proposal_revision": 1, "status": status}
        (package / "domain-change-proposal.json").write_text(json.dumps(proposal), encoding="utf-8")
        evidence = {"scm_attestation": attestation}
        (package / "evidence-bundle.json").write_text(json.dumps(evidence), encoding="utf-8")

    def push_fixture(self) -> tuple[Path, str]:
        amend_policy(self.repo / "memory", "authorized_signers", "SHA256:NOBODY", "A key.")
        amend_policy(self.repo / "memory", "review_trigger", "git-push", "On push.")
        amend_policy(
            self.repo / "memory", "review_verifier", "git-signed-commit", "The maintainer signs."
        )
        self.git("add", "memory")
        self.git("commit", "-qm", "unsigned policy change")
        remote = self.repo / "remote.git"
        self.git("init", "-q", "--bare", str(remote))
        self.git("remote", "add", "origin", str(remote))
        self.git("push", "-q", "origin", "HEAD:refs/heads/main")
        self.git("fetch", "-q", "origin")
        hooks = self.repo / ".git" / "hooks"
        hooks.mkdir(parents=True, exist_ok=True)
        hook = install_pre_push_hook(
            self.repo / "memory",
            self.repo,
            Path(__file__).resolve().parent / "registry_tools.py",
        )
        return hook, self.git("rev-parse", "HEAD")

    def git(self, *args: str) -> str:
        return subprocess.run(
            ["git", "-c", "user.name=T", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", *args],
            cwd=self.repo, capture_output=True, text=True, check=True,
        ).stdout.strip()

    def commit_file(self, relative: str) -> str:
        path = self.repo / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(relative, encoding="utf-8")
        self.git("add", relative)
        self.git("commit", "-qm", f"add {relative}")
        return self.git("rev-parse", "HEAD")

    def run_pre_push(self, hook: Path, refs: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["sh", str(hook), "origin", "unused"],
            cwd=self.repo, input=refs, capture_output=True, text=True, check=False,
        )

    def interpreters_on_path(self, *, python: bool, python3: bool) -> dict[str, str]:
        shims = self.repo / "shims"
        shims.mkdir()
        real = Path(sys.executable).as_posix()
        for name, works in (("python", python), ("python3", python3)):
            body = f'#!/bin/sh\nexec "{real}" "$@"\n' if works else "#!/bin/sh\nexit 1\n"
            shim = shims / name
            shim.write_bytes(body.encode("utf-8"))
            shim.chmod(0o755)
        return {**os.environ, "PATH": str(shims) + os.pathsep + os.environ["PATH"]}

    def pushed_with(self, environment: dict[str, str]) -> subprocess.CompletedProcess:
        hook, _ = self.push_fixture()
        head = self.commit_file("memory/registry/extra.json")
        return subprocess.run(
            ["sh", str(hook), "origin", "unused"], cwd=self.repo, env=environment,
            input=f"refs/heads/feature {head} refs/heads/feature {'0' * 40}\n",
            capture_output=True, text=True, check=False,
        )

    def two_line_rule(self) -> Path:
        (self.repo / "rule.md").write_text("one\ntwo\n", encoding="utf-8")
        return self.repo / "rule.md"

    def two_mapped_contexts(self, **collaborations: list[dict]) -> None:
        self.seed("contexts.json", [
            {"id": "orders", "name": "Orders", "responsibility": "Own orders.", "implementation_path": "src/orders"},
            {"id": "billing", "name": "Billing", "responsibility": "Own invoices.", "implementation_path": "src/billing"},
        ])
        for name, records in collaborations.items():
            self.seed(f"{name.replace('_', '-')}.json", records)
        for path in ("src/orders/port.py", "src/billing/invoice.py"):
            (self.repo / path).parent.mkdir(parents=True, exist_ok=True)
            (self.repo / path).write_text("v1\n", encoding="utf-8")
        self.commit_all("mapped contexts")

    def change(self, *paths: str) -> dict:
        for path in paths:
            (self.repo / path).parent.mkdir(parents=True, exist_ok=True)
            (self.repo / path).write_text("v2\n", encoding="utf-8")
        return boundary_impact(self.repo / "memory", self.repo, "HEAD")

    def interrupted_transaction(self, phase: str, operation_id: str = "op") -> tuple[Path, Path, Path]:
        root = self.repo / "memory"
        backup = root / f".domain-registry-backup-{operation_id}"
        staging = root / f".domain-registry-stage-{operation_id}"
        shutil.copytree(root / "registry", backup)
        staging.mkdir()
        (registry_dir(root) / INDEX).write_text("{}\n", encoding="utf-8")
        transaction_path(root).write_text(
            json.dumps({
                "format": "domain-registry-transaction/v2", "operation_id": operation_id,
                "phase": phase, "backup": backup.name, "staging": staging.name,
            }),
            encoding="utf-8",
        )
        return root, backup, staging

    def probe_signing_key(self) -> dict[str, Any]:
        with patch.dict("os.environ", {SIGNING_KEY_ENV: ""}):
            return init_signing_key(self.repo, "probe@example.com", self.repo / "keys" / "signing-key")

    def head(self) -> str:
        return self.git("rev-parse", "HEAD")

    def draft_package(self) -> Path:
        self.add_context()
        package = self.repo / "proposal"
        init_change_package(package)
        requirement = {
            "format": "domain-requirement/v1",
            "requirement_id": "REQ-1",
            "acceptance_criteria": [
                {"id": "AC-1", "statement": "Orders have a total."}
            ],
            "risk_flags": [],
            "required_approval_roles": ["domain-owner"],
        }
        proposal = {
            "format": "domain-change-proposal/v1",
            "proposal_id": "PRO-1",
            "proposer": "author",
            "requirement_id": "REQ-1",
            "status": "draft",
            "change_classification": "material",
            "submitted_at": None,
            "finalized_at": None,
            "proposal_revision": 1,
            "base_registry_revision": None,
            "affected_contexts": ["orders"],
            "rule_ids": [],
            "contract_ids": [],
            "ownership_decision": {
                "owner_context": "orders",
                "invariant": "Order total is non-negative.",
                "evidence": [],
            },
            "boundary_decision": {
                "interaction": "none",
                "consistency": "synchronous",
                "prohibited_dependencies": [],
                "evidence": [],
            },
            "event_change": None,
            "contract_change": None,
            "registry_updates": [
                {
                    "operation": "upsert",
                    "asset": "rules",
                    "record": {
                        "id": "order-total",
                        "contexts": ["orders"],
                        "statement": "An order total is non-negative.",
                        "evidence": [self.readme_citation()],
                    },
                }
            ],
            "risk_flags": [],
            "open_questions": [],
            "approvals": [],
            "implementation_design": {
                "domain_forces": ["The total must stay non-negative."],
                "decision": "Use the order boundary invariant; no tactical pattern is required.",
                "invariants_preserved": ["Order total is non-negative."],
                "rejected_alternatives": ["Let callers validate totals independently."],
                "proof_obligations": ["OB-1"],
                "counterfactual_check": "Remove the invariant check and expect OB-1 to fail.",
            },
        }
        obligations = {
            "format": "domain-test-obligations/v1",
            "requirement_id": "REQ-1",
            "proposal_id": "PRO-1",
            "obligations": [
                {
                    "id": "OB-1",
                    "source_type": "acceptance-criterion",
                    "source_id": "AC-1",
                    "expected_outcome": "A negative total is rejected.",
                    "check": "test command",
                    "status": "executed",
                }
            ],
        }
        evidence = {
            "format": "domain-evidence-bundle/v1",
            "requirement_id": "REQ-1",
            "proposal_id": "PRO-1",
            "registry_revision": None,
            "approvals": [],
            "scm_attestation": None,
            "counterfactual_check": {
                "status": "passed",
                "obligation_id": "OB-1",
                "mutated_protection": "Remove the non-negative total validation.",
                "failure_evidence": "OB-1 failed with a negative total accepted.",
                "restored": True,
            },
            "sources": [],
            "test_results": [
                {
                    "obligation_id": "OB-1",
                    "status": "passed",
                    "evidence": "test command",
                    "command_profile": "unit",
                    "exit_code": 0,
                    "output_sha256": "sha256:" + "0" * 64,
                    "finished_at": "2026-01-01T00:00:00Z",
                }
            ],
            "contract_checks": [],
            "architecture_checks": [],
            "security_checks": [],
            "residual_risks": [],
        }
        for name, value in (
            ("requirement-normalization.json", requirement),
            ("domain-change-proposal.json", proposal),
            ("test-obligations.json", obligations),
            ("evidence-bundle.json", evidence),
        ):
            (package / name).write_text(json.dumps(value), encoding="utf-8")
        return package

    def written_package(self, **replacements) -> Path:
        package = self.draft_package()
        for name, value in replacements.items():
            path = package / (name.replace("_", "-") + ".json")
            path.write_text(json.dumps(value), encoding="utf-8")
        return package

    def approve_and_apply(self, package: Path) -> None:
        self.approve_and_finalize(package)
        apply_approved_updates(package, self.repo / "memory", self.repo)

    def approve_and_finalize(self, package: Path) -> None:
        submit_proposal(package, self.repo / "memory", self.repo)
        record_approval(package, "domain-owner", "reviewer", "entire proposal", None)
        self.attest_and_finalize(package)

    def attest_and_finalize(self, package: Path) -> None:
        submitted = json.loads(
            (package / "domain-change-proposal.json").read_text(encoding="utf-8")
        )
        evidence_path = package / "evidence-bundle.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["scm_attestation"] = {
            "provider": "git-signed-commit",
            "commit": "a" * 40,
            "status": "approved",
            "proposal_revision": 1,
            "base_registry_revision": submitted["base_registry_revision"],
        }
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        verify_proposal(package, self.repo / "memory", self.repo)
        with patch("domain_registry.hitl.verify_git_signed_commit", return_value=[]), \
                patch("domain_registry.hitl.commit_carries_proposal", return_value=[]):
            finalize_proposal(package, self.repo / "memory", self.repo)

    def redraft_package(
        self, package: Path, proposal_id: str, statement: str, supersedes: str | list[str] | None
    ) -> None:
        for name in (
            "domain-change-proposal.json",
            "test-obligations.json",
            "evidence-bundle.json",
        ):
            path = package / name
            value = json.loads(path.read_text(encoding="utf-8"))
            if "proposal_id" in value:
                value["proposal_id"] = proposal_id
            path.write_text(json.dumps(value), encoding="utf-8")
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal.update(
            {
                "status": "draft",
                "approvals": [],
                "base_registry_revision": None,
                "submitted_at": None,
                "finalized_at": None,
            }
        )
        proposal["registry_updates"][0]["record"]["statement"] = statement
        if supersedes is not None:
            proposal["supersedes"] = supersedes
        path.write_text(json.dumps(proposal), encoding="utf-8")

    def redraft_removal(
        self, package: Path, proposal_id: str, record_id: str, supersedes: str | None
    ) -> None:
        self.redraft_package(package, proposal_id, "unused", supersedes)
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["registry_updates"] = [
            {"operation": "remove", "asset": "rules", "id": record_id}
        ]
        path.write_text(json.dumps(proposal), encoding="utf-8")

    def select_docs_as_sources(self) -> None:
        (self.repo / "docs").mkdir(exist_ok=True)
        (self.repo / "docs" / "decision.md").write_text(
            "the decision\n", encoding="utf-8"
        )
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        write_source_map(
            self.repo / "memory" / "source-map.json",
            {
                "selected_paths": ["docs"],
                "selection_status": "developer-confirmed",
                "source_kinds": {"docs": "decisions"},
                "source_groups": [{"kind": "decisions", "paths": ["docs"]}],
            },
        )

    def cite_in_proposal(self, package: Path, path: str) -> dict:
        proposal_path = package / "domain-change-proposal.json"
        proposal = json.loads(proposal_path.read_text(encoding="utf-8"))
        proposal["registry_updates"][0]["record"]["evidence"] = [
            citation(self.repo, path, 1, 1)
        ]
        proposal_path.write_text(json.dumps(proposal), encoding="utf-8")
        return proposal

    def stored_rule(self) -> dict:
        document = read_document(self.repo / "memory", "rules.json")
        return document["rules"][0]

    def reviewed_rule(self, evidence: list) -> None:
        self.seed(
            "rules.json",
            [
                {
                    "id": "order-total",
                    "contexts": ["orders"],
                    "statement": "An order total is non-negative.",
                    "evidence": evidence,
                    "status": "reviewed",
                }
            ],
        )

    def package_with_a_candidate_rule(self, statement: str) -> Path:
        package = self.draft_package()
        proposal_file = package / "domain-change-proposal.json"
        proposal = json.loads(proposal_file.read_text(encoding="utf-8"))
        proposal["registry_updates"] = []
        proposal_file.write_text(json.dumps(proposal), encoding="utf-8")
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        record = self.write_record(
            "rule.json",
            {"id": "order-total", "contexts": ["orders"], "statement": statement, "evidence": [self.readme_citation()]},
        )
        upsert_candidate(self.repo / "memory", self.repo, "rules", record)
        return package

    def registry_updates(self, package: Path) -> list[dict]:
        proposal = json.loads((package / "domain-change-proposal.json").read_text(encoding="utf-8"))
        return proposal["registry_updates"]

    def redraft_target(self) -> Path:
        package = self.repo / "second"
        shutil.copytree(self.repo / "proposal", package)
        proposal_file = package / "domain-change-proposal.json"
        proposal = json.loads(proposal_file.read_text(encoding="utf-8"))
        proposal["status"] = "draft"
        proposal_file.write_text(json.dumps(proposal), encoding="utf-8")
        return package

    def rule_ids(self) -> list[str]:
        document = read_document(self.repo / "memory", "rules.json")
        return [entry["id"] for entry in document["rules"]]

    def drafted_with_updates(self, updates: list) -> Path:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["registry_updates"] = updates
        path.write_text(json.dumps(proposal), encoding="utf-8")
        return package

    def package_documents(self, package: Path) -> dict[str, dict]:
        return {
            path.name: json.loads(path.read_text(encoding="utf-8"))
            for path in package.glob("*.json")
        }

    def permit_sources(self) -> None:
        path = self.repo / "memory" / "domain-memory-policy.json"
        value = json.loads(path.read_text(encoding="utf-8"))
        value["limits"] |= {
            "max_file_count": 50,
            "max_file_bytes": 100000,
            "max_total_bytes": 500000,
        }
        path.write_text(json.dumps(value), encoding="utf-8")

    def discovery_groups(self, kind: str) -> list[str]:
        return next(
            group
            for group in discover_sources(self.repo)["source_groups"]
            if group["kind"] == kind
        )["paths"]

    def seed_generated_cache(self) -> None:
        (self.repo / "AGENTS.md").write_text("How agents work here.", encoding="utf-8")
        (self.repo / "apps" / "core").mkdir(parents=True)
        (self.repo / "apps" / "core" / "doc.go").write_text(
            "package core\n", encoding="utf-8"
        )
        (self.repo / "apps" / "core" / "core_test.go").write_text(
            "package core\n", encoding="utf-8"
        )
        for cache in (self.repo / ".agents" / "skills", self.repo / "apps" / ".cache"):
            cache.mkdir(parents=True)
        (self.repo / ".agents" / "skills" / "AGENTS.md").write_text(
            "A copy this host generated.", encoding="utf-8"
        )
        (self.repo / "apps" / ".cache" / "doc.go").write_text(
            "package cache\n", encoding="utf-8"
        )
        (self.repo / "apps" / ".cache" / "cached_test.go").write_text(
            "package cache\n", encoding="utf-8"
        )

    def defined_term(self) -> None:
        self.seed(
            "vocabulary.json",
            [
                {
                    "id": "run",
                    "name": "Run",
                    "definition": "One execution of a Skill against a Test Case.",
                    "contexts": ["trial"],
                },
            ],
        )
        self.allow_results(10)

    def look_alike_terms(self) -> tuple[Path, Path]:
        self.add_context()
        return (
            self.write_record("order.json", {"id": "order", "name": "Order", "contexts": ["orders"],
                                             "not_same_as": ["order-line"]}),
            self.write_record("line.json", {"id": "order-line", "name": "Order Line", "contexts": ["orders"],
                                            "not_same_as": ["order"]}),
        )

    def nested_sources(self) -> dict:
        (self.repo / "apps" / "svc").mkdir(parents=True, exist_ok=True)
        (self.repo / "apps" / "other.go").write_text("other", encoding="utf-8")
        (self.repo / "apps" / "svc" / "doc.go").write_text(
            "package svc", encoding="utf-8"
        )
        return selected_source_map(
            self.repo, [self.repo / "apps", self.repo / "apps" / "svc" / "doc.go"]
        )

    def stored_contexts(self) -> list[dict]:
        return read_document(self.repo / "memory", "contexts.json")["contexts"]

    def placement_errors(self, *placements: tuple[str, str]) -> list[str]:
        self.seed(
            "contexts.json",
            [
                {
                    "id": identifier,
                    "name": identifier.title(),
                    "responsibility": f"Own {identifier}.",
                    "implementation_path": path,
                }
                for identifier, path in placements
            ],
        )
        return validate(self.repo / "memory", self.repo, False)

    def run_cli(self, *arguments: str, stdin: str | None = None) -> subprocess.CompletedProcess:
        return subprocess.run(
            [sys.executable, "-m", "domain_registry.cli", *arguments],
            cwd=Path(__file__).parent,
            input=stdin,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env={**os.environ, "PYTHONIOENCODING": "utf-8"},
            check=False,
        )

    def context_declaring(self, asset: str, reason: str) -> None:
        self.seed(
            "contexts.json",
            [
                {
                    "id": "orders",
                    "name": "Orders",
                    "responsibility": "Own orders.",
                    "confirmed_absences": [{"asset": asset, "reason": reason}],
                },
                {"id": "billing", "name": "Billing", "responsibility": "Own invoices."},
            ],
        )

    def submitted_package(self) -> Path:
        package = self.draft_package()
        (self.repo / "README.md").write_text("evidence\n", encoding="utf-8")
        submit_proposal(package, self.repo / "memory", self.repo)
        return package

    def proposal_in(self, package: Path) -> dict:
        return json.loads(
            (package / "domain-change-proposal.json").read_text(encoding="utf-8")
        )

    def stored_policy(self) -> dict:
        return stored_policy_of(self.repo)

    def audit_lines(self) -> list[dict]:
        return [
            json.loads(line)
            for line in (self.repo / "memory" / "audit" / "events.jsonl")
            .read_text(encoding="utf-8")
            .splitlines()
        ]

    def cited_file(self) -> Path:
        path = self.repo / "docs" / "adr" / "boundaries.md"
        path.parent.mkdir(parents=True, exist_ok=True)
        rows = [
            "# Boundaries",
            "Orders owns pricing.",
            "Billing owns invoices.",
            "No trailing newline",
        ]
        path.write_text(chr(10).join(rows), encoding="utf-8")
        return path

    def confirmed_corpus(self) -> None:
        self.cited_file()
        write_source_map(
            self.repo / "memory" / "source-map.json",
            selected_source_map(self.repo, [self.repo / "docs" / "adr"]),
        )

    def counterfactual_verdicts(self, check: dict, status: str = "verified", updates: list | None = None) -> list[str]:
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["status"] = status
        if updates is not None:
            proposal["registry_updates"] = updates
        path.write_text(json.dumps(proposal), encoding="utf-8")
        evidence_path = package / "evidence-bundle.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["counterfactual_check"] = check
        evidence_path.write_text(json.dumps(evidence), encoding="utf-8")
        return [error for error in validate_change_package(package, None) if "counterfactual" in error]

    def vocabulary_update(self) -> list[dict]:
        record = {"id": "order", "name": "Order", "contexts": ["orders"], "definition": "A purchase."}
        return [{"operation": "upsert", "asset": "vocabulary", "record": record}]

    UNPLANNED_CHECK: ClassVar[dict] = {
        "status": "passed", "obligation_id": "OB-2", "mutated_protection": "x", "failure_evidence": "y",
        "restored": True,
    }

    UNPLANNED_MESSAGE = "counterfactual_check must target one of implementation_design's proof_obligations"

    def proposal_reference_errors(self, status: str, **fields: Any) -> list[str]:
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8")) | fields | {"status": status}
        path.write_text(json.dumps(proposal), encoding="utf-8")
        return validate_change_package(package, self.repo / "memory")

    def updated_rule_citing(self, reference: Any) -> Path:
        package = self.draft_package()
        path = package / "domain-change-proposal.json"
        proposal = json.loads(path.read_text(encoding="utf-8"))
        proposal["registry_updates"][0]["record"]["evidence"] = [reference]
        path.write_text(json.dumps(proposal), encoding="utf-8")
        return package

    def evidence_moved(self, old_text: str, new_path: str, new_text: str) -> Path:
        (self.repo / "src").mkdir()
        (self.repo / "src" / "a.py").write_text(old_text, encoding="utf-8")
        reference = citation(self.repo, "src/a.py", 2, 2)
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        self.seed(
            "rules.json",
            [
                {"id": "reviewed-rule", "contexts": ["orders"], "statement": "S.", "status": "reviewed",
                 "evidence": [reference], "verified": VERIFIED},
                {"id": "candidate-rule", "contexts": ["orders"], "statement": "S.", "status": "candidate",
                 "evidence": [reference]},
            ],
        )
        applied_by(self.repo / "memory", "PRO-1", "rules/reviewed-rule")
        (self.repo / "src" / "a.py").unlink()
        target = self.repo / new_path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(new_text, encoding="utf-8")
        return target

    def update_errors(self, updates: Any) -> list[str]:
        package = self.drafted_with_updates(updates)
        return [error for error in validate_change_package(package, self.repo / "memory") if "registry update" in error]

    def marker(self) -> Path:
        return self.repo / "memory" / ".domain-registry-reconciliation.json"

    def pending_marker(self, status: str, applied: bool) -> Path:
        package = self.repo / "proposal"
        package.mkdir()
        proposal_path = package / "domain-change-proposal.json"
        proposal_path.write_text(json.dumps({"proposal_id": "PRO-1", "status": status}), encoding="utf-8")
        self.marker().write_text(json.dumps({"package": "proposal", "proposal_id": "PRO-1"}), encoding="utf-8")
        if applied:
            append_audit(self.repo / "memory", {"operation": "apply-approved-updates", "proposal_id": "PRO-1"})
            append_audit(self.repo / "memory", {"operation": "upsert-candidate", "record_id": "later"})
        return proposal_path

    def cited_rule(self, status: str, *spans: tuple[int, int]) -> None:
        (self.repo / "src").mkdir()
        (self.repo / "src" / "a.py").write_text("one\ntwo\nthree\n", encoding="utf-8")
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        rule = {
            "id": "rule", "contexts": ["orders"], "statement": "S.", "status": status,
            "evidence": [citation(self.repo, "src/a.py", start, end) for start, end in spans],
            "verified": VERIFIED,
        }
        self.seed("rules.json", [rule])
        applied_by(self.repo / "memory", "PRO-1", "rules/rule")
        (self.repo / "src" / "a.py").write_text("zero\none\ntwo\nthree\n", encoding="utf-8")

    def citation_after_edit(self, before: str, cited: int, after: str, last: int | None = None) -> dict:
        source = self.repo / "fact.md"
        source.write_text(before, encoding="utf-8")
        reference = citation(self.repo, "fact.md", cited, last or cited)
        source.write_text(after, encoding="utf-8")
        return verify(reference, self.repo)

    def seed_two_cited_rules(self) -> None:
        (self.repo / "fact.md").write_text("a\nfact\n", encoding="utf-8")
        (self.repo / "other.md").write_text("other\n", encoding="utf-8")
        self.seed("contexts.json", [{"id": "orders", "name": "Orders", "responsibility": "Own orders."}])
        self.seed(
            "rules.json",
            [
                {"id": "moved", "contexts": ["orders"], "statement": "S.", "status": "reviewed",
                 "evidence": [citation(self.repo, "fact.md", 2, 2)], "verified": VERIFIED},
                {"id": "kept", "contexts": ["orders"], "statement": "S.", "status": "reviewed",
                 "evidence": [citation(self.repo, "fact.md", 1, 1), citation(self.repo, "other.md", 1, 1)],
                 "verified": VERIFIED},
            ],
        )
        applied_by(self.repo / "memory", "PRO-1", "rules/moved")
        applied_by(self.repo / "memory", "PRO-2", "rules/kept")
        (self.repo / "fact.md").write_text("a\nnew\nfact\n", encoding="utf-8")

    def contexts_mentioning_suggestions(self) -> None:
        self.allow_results(10)
        self.seed(
            "contexts.json",
            [
                {"id": "eval", "name": "Eval", "responsibility": "Applies accepted Suggestions.", "status": "reviewed"},
                {"id": "draft", "name": "Draft", "responsibility": "Drafts suggestions.", "status": "candidate"},
            ],
        )

    def sign_a_commit_changing(self, relative: str) -> str:
        path = self.repo / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(relative, encoding="utf-8")
        subprocess.run(["git", "add", relative], cwd=self.repo, check=True)
        subprocess.run(
            ["git", "-c", "user.name=Probe", "-c", "user.email=probe@example.com",
             "commit", "-qS", "-m", f"change {relative}"],
            cwd=self.repo, check=True,
        )
        return self.head()
