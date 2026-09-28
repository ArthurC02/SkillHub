from __future__ import annotations

import argparse
import ast
import json
import re
import shutil
import subprocess
import sys
import tempfile
from collections.abc import Callable, Iterator
from dataclasses import dataclass
from pathlib import Path

REPORT = "REPORT.md"
TOOL_CONFIGS = (
    "pyproject.toml",
    "setup.cfg",
    "tox.ini",
    "ruff.toml",
    ".ruff.toml",
    ".flake8",
    ".pylintrc",
    ".pre-commit-config.yaml",
)
PROVIDER_TERMS = re.compile(r"textline|urllib|http\.client|\b202\b|\b429\b", re.IGNORECASE)
ORDER_DECISIONS = re.compile(r"^class Order\b|^def confirm_order\b", re.MULTILINE)
ABSTRACTION_MARKERS = re.compile(r"\bProtocol\b|\bABC\b|\babstractmethod\b")
ROUNDING_MECHANICS = re.compile(r"\bquantize\b|\bROUND_[A-Z_]+\b")
NESTING_NODES = (ast.If, ast.For, ast.While, ast.Try, ast.With, ast.Match)

Check = dict[str, object]


@dataclass(frozen=True)
class Run:
    fixture: Path
    result: Path


def verdict(name: str, *, passed: bool | None, detail: object = "") -> Check:
    return {"check": name, "passed": passed, "detail": detail}


def product_files(root: Path) -> list[Path]:
    return sorted(
        path
        for path in root.rglob("*.py")
        if "tests" not in path.relative_to(root).parts and "__pycache__" not in path.parts
    )


def tracked_files(root: Path) -> set[str]:
    return {
        path.relative_to(root).as_posix()
        for path in root.rglob("*")
        if path.is_file() and "__pycache__" not in path.parts
    }


def run_python(root: Path, *arguments: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, *arguments],
        cwd=root,
        capture_output=True,
        text=True,
        encoding="utf-8",
        timeout=120,
        check=False,
    )


def run_tests(root: Path) -> subprocess.CompletedProcess[str]:
    return run_python(root, "-m", "unittest", "discover", "-s", "tests", "-t", ".")


def test_names(root: Path) -> set[str]:
    names = set()
    for path in (root / "tests").rglob("*.py"):
        tree = ast.parse(path.read_text(encoding="utf-8"))
        names.update(
            node.name
            for node in ast.walk(tree)
            if isinstance(node, ast.FunctionDef) and node.name.startswith("test_")
        )
    return names


def tests_pass(run: Run) -> Check:
    outcome = run_tests(run.result)
    return verdict("tests_pass", passed=outcome.returncode == 0, detail=outcome.stderr[-400:])


def existing_tests_kept(run: Run) -> Check:
    missing = sorted(test_names(run.fixture) - test_names(run.result))
    return verdict("existing_tests_kept", passed=not missing, detail=missing)


def no_new_tool_config(run: Run) -> Check:
    added = sorted(
        name
        for name in tracked_files(run.result) - tracked_files(run.fixture)
        if Path(name).name in TOOL_CONFIGS
    )
    return verdict("no_new_tool_config", passed=not added, detail=added)


def provider_stays_out_of_the_decision(run: Run) -> Check:
    leaking = [
        path.relative_to(run.result).as_posix()
        for path in product_files(run.result)
        if ORDER_DECISIONS.search(text := path.read_text(encoding="utf-8"))
        and PROVIDER_TERMS.search(text)
    ]
    return verdict("provider_stays_out_of_the_decision", passed=not leaking, detail=leaking)


def imported_modules(path: Path) -> set[str]:
    tree = ast.parse(path.read_text(encoding="utf-8"))
    names = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            names.update(alias.name for alias in node.names)
        if isinstance(node, ast.ImportFrom) and node.module:
            names.add(node.module)
    return names


def project_imports(root: Path, start: Path) -> set[Path]:
    reached: set[Path] = set()
    pending = [start]
    while pending:
        for module in imported_modules(pending.pop()):
            target = root / (module.replace(".", "/") + ".py")
            if target.is_file() and target not in reached:
                reached.add(target)
                pending.append(target)
    return reached


def decision_does_not_import_the_provider(run: Run) -> Check:
    chains = [
        f"{path.relative_to(run.result).as_posix()} -> {reached.relative_to(run.result).as_posix()}"
        for path in product_files(run.result)
        if ORDER_DECISIONS.search(path.read_text(encoding="utf-8"))
        for reached in sorted(project_imports(run.result, path))
        if PROVIDER_TERMS.search(reached.read_text(encoding="utf-8"))
    ]
    return verdict("decision_does_not_import_the_provider", passed=not chains, detail=chains)


def provider_is_named_somewhere(run: Run) -> Check:
    holders = [
        path.relative_to(run.result).as_posix()
        for path in product_files(run.result)
        if PROVIDER_TERMS.search(path.read_text(encoding="utf-8"))
    ]
    return verdict("provider_is_named_somewhere", passed=bool(holders), detail=holders)


DISCOUNT_TABLE = (
    ("100", 0, "100.00"),
    ("100", 9, "100.00"),
    ("100", 10, "95.00"),
    ("100", 24, "95.00"),
    ("100", 25, "92.00"),
    ("10.05", 10, "9.55"),
)
DISCOUNT_PROBE = """
import json, sys
from decimal import Decimal
from shop.pricing import discounted_total
table = json.loads(sys.argv[1])
print(json.dumps([str(discounted_total(Decimal(t), n)) for t, n, _ in table]))
"""


def discount_applies_at_its_thresholds(run: Run) -> Check:
    outcome = run_python(run.result, "-c", DISCOUNT_PROBE, json.dumps(DISCOUNT_TABLE))
    if outcome.returncode != 0:
        return verdict(
            "discount_applies_at_its_thresholds", passed=False, detail=outcome.stderr[-400:]
        )
    wrong = [
        {"total": total, "past_orders": past, "expected": expected, "got": got}
        for (total, past, expected), got in zip(
            DISCOUNT_TABLE, json.loads(outcome.stdout), strict=True
        )
        if got != expected
    ]
    return verdict("discount_applies_at_its_thresholds", passed=not wrong, detail=wrong)


DISCOUNT_THRESHOLDS = (10, 25)


def thresholds_have_names(run: Run) -> Check:
    anonymous = sorted(
        {
            f"{function.name}: {node.value}"
            for function in functions(run.result)
            for node in ast.walk(function)
            if isinstance(node, ast.Constant)
            and type(node.value) is int
            and node.value in DISCOUNT_THRESHOLDS
        }
    )
    return verdict("thresholds_have_names", passed=not anonymous, detail=anonymous)


def moved_by_one(source: str, threshold: int) -> str:
    return re.sub(rf"\b{threshold}\b", str(threshold + 1), source)


def a_test_sits_on_each_threshold(run: Run) -> Check:
    survivors = []
    with tempfile.TemporaryDirectory() as temporary:
        copy = Path(temporary) / "copy"
        shutil.copytree(run.result, copy, ignore=shutil.ignore_patterns("__pycache__"))
        sources = {path: path.read_text(encoding="utf-8") for path in product_files(copy)}
        for threshold in DISCOUNT_THRESHOLDS:
            for path, source in sources.items():
                path.write_text(moved_by_one(source, threshold), encoding="utf-8")
            if run_tests(copy).returncode == 0:
                survivors.append(threshold)
    return verdict("a_test_sits_on_each_threshold", passed=not survivors, detail=survivors)


def files_matching(root: Path, pattern: re.Pattern[str], skip: str) -> list[str]:
    return [
        path.relative_to(root).as_posix()
        for path in product_files(root)
        if path.relative_to(root).as_posix() != skip
        and pattern.search(path.read_text(encoding="utf-8"))
    ]


def rounding_is_reused(run: Run) -> Check:
    copies = files_matching(run.result, ROUNDING_MECHANICS, "shop/money.py")
    return verdict("rounding_is_reused", passed=not copies, detail=copies)


def no_abstraction_without_a_second_case(run: Run) -> Check:
    holders = files_matching(run.result, ABSTRACTION_MARKERS, "")
    return verdict("no_abstraction_without_a_second_case", passed=not holders, detail=holders)


QUOTE_PROBE = """
import itertools, json
from decimal import Decimal
from shipping.quote import QuoteRefused, Shipment, quote
grid = itertools.product(
    ("air", "ground", "sea"),
    ("0", "2", "10.5", "60"),
    (50, 120, 121, 200, 201),
    ("10001", "90001"),
    ("99.99", "100"),
    (False, True),
)
answers = []
for service, weight, side, postcode, value, hazardous in grid:
    shipment = Shipment(
        service, Decimal(weight), side, postcode, Decimal(value), hazardous
    )
    try:
        answers.append(str(quote(shipment)))
    except QuoteRefused as refusal:
        answers.append("refused:" + str(refusal))
print(json.dumps(answers))
"""


def quotes_unchanged(run: Run) -> Check:
    before = run_python(run.fixture, "-c", QUOTE_PROBE)
    after = run_python(run.result, "-c", QUOTE_PROBE)
    if after.returncode != 0:
        return verdict("quotes_unchanged", passed=False, detail=after.stderr[-400:])
    expected, got = json.loads(before.stdout), json.loads(after.stdout)
    changed = sum(1 for a, b in zip(expected, got, strict=True) if a != b)
    return verdict(
        "quotes_unchanged", passed=changed == 0, detail={"cases": len(expected), "changed": changed}
    )


def functions(root: Path) -> Iterator[ast.FunctionDef]:
    for path in product_files(root):
        tree = ast.parse(path.read_text(encoding="utf-8"))
        yield from (n for n in ast.walk(tree) if isinstance(n, ast.FunctionDef))


def nesting_depth(node: ast.AST, depth: int = 0) -> int:
    deepest = depth
    for child in ast.iter_child_nodes(node):
        step = 1 if isinstance(child, NESTING_NODES) else 0
        deepest = max(deepest, nesting_depth(child, depth + step))
    return deepest


def function_length(node: ast.FunctionDef) -> int:
    return (node.end_lineno or node.lineno) - node.lineno + 1


def shape(root: Path) -> dict[str, int]:
    found = list(functions(root))
    return {
        "functions": len(found),
        "deepest_nesting": max((nesting_depth(f) for f in found), default=0),
        "longest_function": max((function_length(f) for f in found), default=0),
        "one_statement_functions": sum(1 for f in found if len(f.body) == 1),
    }


def nesting_went_down(run: Run) -> Check:
    before, after = shape(run.fixture), shape(run.result)
    return verdict(
        "nesting_went_down",
        passed=after["deepest_nesting"] < before["deepest_nesting"],
        detail={"before": before, "after": after},
    )


QUOTE_MUTANTS = {
    "remote_area_fee_cap": (
        't = Decimal("12.00")',
        't = Decimal("13.00")',
    ),
    "oversize_limit": ("s.longest_side_cm > 200", "s.longest_side_cm > 201"),
    "free_shipping_threshold": (
        's.order_value >= Decimal("100")',
        's.order_value > Decimal("100")',
    ),
    "oversize_surcharge_starts": (
        "s.longest_side_cm > 120",
        "s.longest_side_cm > 121",
    ),
}


def original_with_tests_of(fixture: Path, result: Path, workspace: Path) -> Path:
    shutil.copytree(fixture, workspace, ignore=shutil.ignore_patterns("__pycache__", "tests"))
    shutil.copytree(
        result / "tests", workspace / "tests", ignore=shutil.ignore_patterns("__pycache__")
    )
    return workspace


def rules_are_pinned_by_tests(run: Run) -> Check:
    survivors = []
    with tempfile.TemporaryDirectory() as temporary:
        clean = original_with_tests_of(run.fixture, run.result, Path(temporary) / "clean")
        if run_tests(clean).returncode != 0:
            return verdict(
                "rules_are_pinned_by_tests",
                passed=None,
                detail="the tests do not run against the original through its public names",
            )
        source = (clean / "shipping" / "quote.py").read_text(encoding="utf-8")
        for name, (old, new) in QUOTE_MUTANTS.items():
            (clean / "shipping" / "quote.py").write_text(source.replace(old, new), encoding="utf-8")
            if run_tests(clean).returncode == 0:
                survivors.append(name)
    return verdict("rules_are_pinned_by_tests", passed=not survivors, detail=survivors)


RELEASE_PROBE = """
from stock.reservations import StockItem
item = StockItem("sku-1", 5)
item.reserve("o-1", 2)
item.reserve("o-2", 1)
item.release("o-1")
item.release("o-1")
print(item.available())
"""


def release_twice_gives_back_once(run: Run) -> Check:
    outcome = run_python(run.result, "-c", RELEASE_PROBE)
    return verdict(
        "release_twice_gives_back_once",
        passed=outcome.returncode == 0 and outcome.stdout.strip() == "4",
        detail=(outcome.stdout + outcome.stderr)[-400:],
    )


def renamed(text: str) -> str:
    return re.sub(r"\bcalc\b", "invoice_total", text)


def only_the_name_changed(run: Run) -> Check:
    differing = [
        name
        for name in sorted(tracked_files(run.fixture))
        if name.endswith(".py")
        and (
            not (run.result / name).is_file()
            or (run.result / name).read_text(encoding="utf-8")
            != renamed((run.fixture / name).read_text(encoding="utf-8"))
        )
    ]
    return verdict("only_the_name_changed", passed=not differing, detail=differing)


def no_file_added(run: Run) -> Check:
    added = sorted(tracked_files(run.result) - tracked_files(run.fixture) - {REPORT})
    return verdict("no_file_added", passed=not added, detail=added)


def files_added(run: Run) -> Check:
    added = sorted(tracked_files(run.result) - tracked_files(run.fixture) - {REPORT})
    return verdict("files_added", passed=None, detail=added)


def shape_measured(run: Run) -> Check:
    return verdict(
        "shape_measured",
        passed=None,
        detail={"before": shape(run.fixture), "after": shape(run.result)},
    )


Checker = Callable[[Run], Check]
EVERY_SCENARIO: tuple[Checker, ...] = (
    tests_pass,
    existing_tests_kept,
    no_new_tool_config,
    files_added,
    shape_measured,
)
SCENARIOS: dict[str, tuple[Checker, ...]] = {
    "receipt-by-sms": (
        provider_is_named_somewhere,
        provider_stays_out_of_the_decision,
        decision_does_not_import_the_provider,
    ),
    "loyalty-discount": (
        discount_applies_at_its_thresholds,
        thresholds_have_names,
        a_test_sits_on_each_threshold,
        rounding_is_reused,
        no_abstraction_without_a_second_case,
    ),
    "shipping-quote": (
        quotes_unchanged,
        nesting_went_down,
        rules_are_pinned_by_tests,
    ),
    "release-reservation": (release_twice_gives_back_once,),
    "rename-calc": (only_the_name_changed, no_file_added),
}


def check(scenario: str, run: Run) -> list[Check]:
    return [checker(run) for checker in EVERY_SCENARIO + SCENARIOS[scenario]]


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Run the machine checks of one code-expression scenario."
    )
    parser.add_argument("--scenario", required=True, choices=sorted(SCENARIOS))
    parser.add_argument("--result", required=True, type=Path)
    args = parser.parse_args()
    fixture = Path(__file__).parent / "fixtures" / args.scenario
    checks = check(args.scenario, Run(fixture.resolve(), args.result.resolve()))
    print(json.dumps({"scenario": args.scenario, "checks": checks}, indent=2))
    return 1 if any(c["passed"] is False for c in checks) else 0


if __name__ == "__main__":
    raise SystemExit(main())
