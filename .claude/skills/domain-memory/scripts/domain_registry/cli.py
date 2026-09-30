from .cli_commands import execute
from .cli_parser import build_parser


def main() -> int:
    try:
        return execute(build_parser().parse_args())
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
