#!/usr/bin/env python3
"""Stdlib fallback harness for scripts/sweep/test_runner.py.

Pytest is not installed in this environment (confirmed 2026-08-01:
`python3 -m pytest` -> No module named pytest). The test module is a flat
collection of zero-argument `test_*` functions using plain `assert`, so this
harness discovers and runs them without adding a dependency. It exists so
modifications to the sweep harness remain tested rather than merely planned:
the project's standing rule requires executed verification for every change,
and absence of a tool must be compensated by other evidence, never silent
approval.

If pytest becomes available, prefer it; this harness intentionally covers
only the zero-argument function style used by test_runner.py.
"""
import inspect
import importlib.util
import os
import sys
import traceback


def main():
    here = os.path.dirname(os.path.abspath(__file__))
    path = os.path.join(here, "test_runner.py")
    spec = importlib.util.spec_from_file_location("test_runner", path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)

    tests = [
        (name, fn)
        for name, fn in sorted(vars(mod).items())
        if name.startswith("test_") and callable(fn)
        and len(inspect.signature(fn).parameters) == 0
    ]
    if not tests:
        print("no zero-argument test_* functions found", file=sys.stderr)
        return 1

    failures = 0
    for name, fn in tests:
        try:
            fn()
        except Exception:
            failures += 1
            print(f"FAIL {name}")
            traceback.print_exc()
        else:
            print(f"ok   {name}")
    print(f"{len(tests) - failures}/{len(tests)} passed")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
