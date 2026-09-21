from .cli_commands import execute
from .cli_parser import build_parser


def main() -> int:
    args = build_parser().parse_args()
    return execute(args)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except ValueError as error:
        print(f"ERROR: {error}")
        raise SystemExit(1) from error
