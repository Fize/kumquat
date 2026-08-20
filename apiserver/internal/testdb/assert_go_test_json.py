#!/usr/bin/env python3
"""Reject any Go test skip event from an authoritative JSON test log."""

import json
import pathlib
import sys


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: assert_go_test_json.py <go-test.json>", file=sys.stderr)
        return 2
    skipped = []
    for line_number, line in enumerate(pathlib.Path(sys.argv[1]).read_text().splitlines(), 1):
        try:
            event = json.loads(line)
        except json.JSONDecodeError as exc:
            print(f"invalid go test JSON at line {line_number}: {exc}", file=sys.stderr)
            return 2
        if event.get("Action") == "skip":
            skipped.append(event.get("Test") or event.get("Package") or "unknown")
    if skipped:
        print("unexpected skipped Go tests/packages: " + ", ".join(skipped), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
