#!/usr/bin/env python3
import contextlib
import hashlib
import io
import json
import shutil
import sys
import tempfile
from pathlib import Path
from unittest import mock

import validate_packaging as packaging

DIGEST = "5d41402abc4b2a76b9719d911017c592ab6f1e2d3c4b5a69788796a5b4c3d2e1"


def run_main(**patches):
    out = io.StringIO()
    with contextlib.ExitStack() as stack:
        for name, value in patches.items():
            stack.enter_context(mock.patch.object(packaging, name, value))
        stack.enter_context(contextlib.redirect_stdout(out))
        code = packaging.main()
    return code, out.getvalue()


def broken_contracts(folder):
    root = Path(folder)
    manifest = json.loads((packaging.PACKAGING_DIR / packaging.MANIFEST).read_text(encoding="utf-8"))
    manifest["examples"] = manifest["examples"][:1]
    del manifest["examples"][0]["manifest_hash"]
    manifest["$defs"]["manifestHashInput"]["examples"] = [{"SKILL.md": DIGEST}, {"SKILL.md": "x"}]
    (root / packaging.MANIFEST).write_text(json.dumps(manifest), encoding="utf-8")
    shutil.copy(packaging.PACKAGING_DIR / packaging.PROFILE, root / packaging.PROFILE)
    test_case = json.loads((packaging.PACKAGING_DIR / packaging.TEST_CASE).read_text(encoding="utf-8"))
    test_case["examples"] = []
    (root / packaging.TEST_CASE).write_text(json.dumps(test_case), encoding="utf-8")
    profiles = root / "profiles"
    profiles.mkdir()
    shutil.copy(packaging.PROFILES_DIR / "standard.json", profiles / "standard.json")
    return dict(
        PACKAGING_DIR=root, PROFILES_DIR=profiles,
        NEGATIVE_CASES=[(packaging.PROFILE, "a valid profile", packaging._profile()),
                        packaging.NEGATIVE_CASES[0]],
        HASH_INPUT_NEGATIVE_CASES=[("a valid map", {"SKILL.md": DIGEST}),
                                   packaging.HASH_INPUT_NEGATIVE_CASES[0]],
    )


def test_the_committed_contracts_still_validate_with_the_same_report():
    code, out = run_main()
    assert code == 0
    assert out.splitlines()[-1] == "3 schemas, 9 examples, 22 counterexamples, 0 failure(s)", out
    assert hashlib.sha256(out.encode()).hexdigest() == EXPECTED_REAL_DIGEST, out


def test_every_kind_of_defect_is_reported_and_fails_the_run():
    with tempfile.TemporaryDirectory() as folder:
        code, out = run_main(**broken_contracts(folder))
    assert code == 1
    assert out == EXPECTED_BROKEN_REPORT, out


EXPECTED_REAL_DIGEST = "68249767f05aa50740dcf41d13ebfb90f0309c8a4ed0226f7e054dff6f21212f"
EXPECTED_BROKEN_REPORT = (
    "FAIL  example download-manifest.schema.json[0]\n"
    "        /: 'manifest_hash' is a required property\n"
    "ok    example manifestHashInput[0]\n"
    "FAIL  example manifestHashInput[1]\n"
    "        /SKILL.md: 'x' does not match '^[0-9a-f]{64}$'\n"
    "FAIL  counterexample accepted: a valid map\n"
    "ok    counterexample rejected: the manifest hashing itself\n"
    "FAIL  profiles/ declares ('standard',), expected ('claude-agent-sdk', 'claude-code', 'standard')\n"
    "ok    example profiles/standard.json\n"
    "FAIL  portable-test-case.schema.json has nothing to validate against\n"
    "FAIL  counterexample accepted: a valid profile\n"
    "ok    counterexample rejected: a licence expression with no provenance tier\n"
    "\n3 schemas, 4 examples, 4 counterexamples, 6 failure(s)\n"
)


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
