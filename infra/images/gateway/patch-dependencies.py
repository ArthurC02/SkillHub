import importlib.metadata
import re
import subprocess
import sys

KNOWN = "Found 1 incompatibility\nThe package `litellm-admin-mcp` requires `mcp==2.2.0`, but `2.3.0` is installed"


def validate(code, output):
    result = code, output.strip()
    if result not in [(0, ""), (1, KNOWN)]:
        raise ValueError(f"unexpected dependency check: {result}")
    return result


def check():
    result = subprocess.run(
        ["/tmp/uv", "pip", "check", "--quiet", "--no-config", "--python", sys.executable],
        capture_output=True,
        text=True,
        check=False,
    )
    print(result.stdout + result.stderr, flush=True)
    return validate(result.returncode, result.stdout + result.stderr)


def inventory():
    return {
        re.sub(r"[-_.]+", "-", item.metadata["Name"]).lower(): item.version
        for item in importlib.metadata.distributions()
    }


def patch():
    before = inventory()
    report = check()
    subprocess.run(
        [
            "/tmp/uv",
            "pip",
            "install",
            "--python",
            sys.executable,
            "--no-deps",
            "--require-hashes",
            "--only-binary",
            ":all:",
            "--no-cache",
            "-r",
            "/fixture/requirements.txt",
        ],
        check=True,
    )
    after = inventory()
    if before.keys() != after.keys() or any(
        before[name] != after[name] for name in before.keys() - {"filelock", "oauthlib"}
    ):
        raise ValueError("unexpected runtime package change")
    if check() != report:
        raise ValueError("dependency incompatibilities changed")
    print("Only the two selected packages changed; dependency check result retained", flush=True)


def self_test():
    assert validate(0, "") == (0, "")
    assert validate(1, KNOWN + "\n") == (1, KNOWN)
    for code, output in [(0, KNOWN), (1, ""), (1, KNOWN + "\nother conflict"), (2, "")]:
        try:
            validate(code, output)
        except ValueError:
            continue
        raise AssertionError("accepted an unexpected dependency result")
    print("dependency gate: six self-checks passed")


if __name__ == "__main__":
    if sys.argv[1:] == ["--self-test"]:
        self_test()
    elif sys.argv[1:]:
        raise SystemExit("usage: patch-dependencies.py [--self-test]")
    else:
        patch()
