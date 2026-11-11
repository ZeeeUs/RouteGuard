"""Fail a release whose engine APK exceeds the recorded development budget."""

import json
import pathlib
import sys

root = pathlib.Path(__file__).resolve().parent.parent
limit = json.loads((root / "tests/resource-budgets.json").read_text())["engine_apk_bytes"]
size = (pathlib.Path(sys.argv[1]) / "routeguard-aarch64_cortex-a53.apk").stat().st_size
if size > limit:
    raise SystemExit(f"Engine APK is {size} bytes, exceeding the {limit}-byte budget")
print(f"Engine APK: {size} bytes (budget: {limit})")
