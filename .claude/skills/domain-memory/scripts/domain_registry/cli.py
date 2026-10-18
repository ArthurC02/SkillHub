import io
import subprocess
import sys

from .cli_commands import COUNTERFACTUAL_EXIT_CODES, COUNTERFACTUAL_FORMAT, execute, print_json
from .cli_parser import UsageError, build_parser


def main() -> int:
    for stream in (sys.stdout, sys.stderr):
        if isinstance(stream, io.TextIOWrapper):
            stream.reconfigure(encoding="utf-8", errors="backslashreplace", newline="\n")
    try:
        args = build_parser().parse_args()
    except UsageError as error:
        return _refuse_usage(error)
    try:
        return execute(args)
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        print(f"ERROR: {error}", file=sys.stderr)
        return 1


def _refuse_usage(error: UsageError) -> int:
    if sys.argv[1:2] == ["counterfactual"]:
        print_json({"format": COUNTERFACTUAL_FORMAT, "verdict": "error", "reason": str(error)})
        return COUNTERFACTUAL_EXIT_CODES["error"]
    sys.stderr.write(f"{error.usage}{error}\n")
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
