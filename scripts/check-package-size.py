"""Fail a release whose engine APK exceeds the recorded development budget."""

import json
import pathlib
import sys

root = pathlib.Path(__file__).resolve().parent.parent
limit = json.loads((root / "tests/resource-budgets.json").read_text())["engine_apk_bytes"]
packages = sorted(pathlib.Path(sys.argv[1]).glob("routeguard-*.apk"))
if not packages:
    raise SystemExit("No engine APKs found")
for package in packages:
    size = package.stat().st_size
    if size > limit:
        raise SystemExit(f"{package.name} is {size} bytes, exceeding the {limit}-byte budget")
    print(f"{package.name}: {size} bytes (budget: {limit})")
