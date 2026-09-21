from domain_registry.cli import main

if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except ValueError as error:
        print(f"ERROR: {error}")
        raise SystemExit(1) from error
