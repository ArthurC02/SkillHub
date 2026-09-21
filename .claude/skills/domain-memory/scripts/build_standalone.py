from __future__ import annotations

import argparse
import shutil
from pathlib import Path


def build(plugin_root: Path, skill_name: str, output: Path) -> None:
    skill_root = plugin_root / "skills" / skill_name
    if not (skill_root / "SKILL.md").is_file():
        raise ValueError(f"unknown Skill: {skill_name}")
    if output.exists() and any(output.iterdir()):
        raise ValueError(f"output directory is not empty: {output}")
    output.mkdir(parents=True, exist_ok=True)
    skill = (skill_root / "SKILL.md").read_text(encoding="utf-8")
    skill = skill.replace("../../references/", "references/")
    (output / "SKILL.md").write_text(skill, encoding="utf-8")
    for name in ("references", "templates"):
        shutil.copytree(plugin_root / name, output / name, dirs_exist_ok=True)
    shutil.copytree(
        plugin_root / "scripts",
        output / "scripts",
        dirs_exist_ok=True,
        ignore=shutil.ignore_patterns(
            "__pycache__", "test_*.py", "build_standalone.py"
        ),
    )


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Build a self-contained standalone Domain Memory Skill."
    )
    parser.add_argument("--plugin-root", required=True, type=Path)
    parser.add_argument("--skill", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    build(args.plugin_root.resolve(), args.skill, args.output.resolve())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
