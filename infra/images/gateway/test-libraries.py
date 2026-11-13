import importlib.metadata
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import xml.etree.ElementTree as ET

PACKAGES = {"filelock": ("4.2.9", 3034), "oauthlib": ("4.0.1", 719)}
PROOF = Path("/tmp/library-proof.json")


def validate_versions(installed):
    if any(installed.get(name) != version for name, (version, _) in PACKAGES.items()):
        raise ValueError("upstream test sources differ from installed package versions")


def prepare():
    installed = {item.metadata["Name"]: item.version for item in importlib.metadata.distributions()}
    validate_versions(installed)
    Path("/tmp/runtime-inventory.json").write_text(
        json.dumps(
            {
                "python": installed,
                "apk_database_sha256": hashlib.sha256(
                    Path("/lib/apk/db/installed").read_bytes()
                ).hexdigest(),
            }
        )
    )
    Path("/tmp/runtime-constraints.txt").write_text(
        "".join(f"{name}=={version}\n" for name, version in sorted(installed.items()))
    )
    for name, (version, _) in PACKAGES.items():
        with tarfile.open(f"/tmp/{name}.tar.gz") as archive:
            archive.extractall("/tmp/upstream", filter="data")
        source = Path(f"/tmp/upstream/{name}-{version}")
        shutil.copytree(
            source if name == "filelock" else source / "tests",
            Path("/tmp/tests") / name / ("" if name == "filelock" else "tests"),
        )


def report(data, expected):
    root = ET.fromstring(data)
    cases = root.findall(".//testcase")
    skipped = len(root.findall(".//skipped"))
    if (
        len(cases) != expected
        or skipped == expected
        or root.findall(".//failure")
        or root.findall(".//error")
    ):
        raise ValueError("incomplete or failed upstream test report")
    return {"passed": expected - skipped, "skipped": skipped}


def run():
    before = json.loads(Path("/tmp/runtime-inventory.json").read_text())["python"]
    if any(importlib.metadata.version(name) != version for name, version in before.items()):
        raise ValueError("test dependencies changed the runtime packages")
    results = {}
    for name, (_, expected) in PACKAGES.items():
        output = f"/tmp/{name}.xml"
        args = ["tests", "-q", "-ra", f"--junitxml={output}"]
        if name == "filelock":
            for plugin in [
                "pytest_asyncio.plugin",
                "pytest_mock",
                "pytest_timeout",
                "pytest_cov",
                "pytest_randomly",
            ]:
                args.extend(["-p", plugin])
        code = (
            "import importlib,pytest; m=importlib.import_module(%r); print('Runtime module:',m.__file__,flush=True); assert m.__file__.startswith('/app/.venv/lib/python3.13/site-packages/'); raise SystemExit(pytest.main(%r))"
            % (name, args)
        )
        subprocess.run(
            [sys.executable, "-B", "-c", code],
            cwd=f"/tmp/tests/{name}",
            env=dict(os.environ, PYTEST_DISABLE_PLUGIN_AUTOLOAD="1"),
            check=True,
        )
        results[name] = report(Path(output).read_text(), expected)
    PROOF.write_text(json.dumps(results))
    print(json.dumps(results), flush=True)


def verify():
    before = json.loads(Path("/tmp/runtime-inventory.json").read_text())
    installed = {item.metadata["Name"]: item.version for item in importlib.metadata.distributions()}
    validate_versions(installed)
    if (
        installed != before["python"]
        or hashlib.sha256(Path("/lib/apk/db/installed").read_bytes()).hexdigest()
        != before["apk_database_sha256"]
    ):
        raise ValueError("shipping runtime differs from the tested runtime")
    results = json.loads(PROOF.read_text())
    if results.keys() != PACKAGES.keys() or any(
        value["passed"] <= 0 or value["passed"] + value["skipped"] != PACKAGES[name][1]
        for name, value in results.items()
    ):
        raise ValueError("invalid upstream test proof")
    print(json.dumps(results), flush=True)


def self_test():
    validate_versions({"filelock": "4.2.9", "oauthlib": "4.0.1"})
    for installed in [
        {},
        {"filelock": "4.2.0", "oauthlib": "4.0.1"},
        {"filelock": "4.2.9", "oauthlib": "4.0.0"},
    ]:
        try:
            validate_versions(installed)
        except ValueError:
            continue
        raise AssertionError("accepted mismatched upstream test sources")
    assert report("<testsuite><testcase/><testcase><skipped/></testcase></testsuite>", 2) == {
        "passed": 1,
        "skipped": 1,
    }
    for data in [
        "<testsuite/>",
        "<testsuite><testcase/></testsuite>",
        "<testsuite><testcase><skipped/></testcase><testcase><skipped/></testcase></testsuite>",
        "<testsuite><testcase><failure/></testcase><testcase/></testsuite>",
        "<testsuite><testcase><error/></testcase><testcase/></testsuite>",
    ]:
        try:
            report(data, 2)
        except ValueError:
            continue
        raise AssertionError("accepted an incomplete upstream test report")
    print("test source and reporter: ten self-checks passed")


if __name__ == "__main__":
    {"prepare": prepare, "run": run, "verify": verify, "--self-test": self_test}[sys.argv[1]]()
