"""Run inside the isolated integration namespaces, with manual.uci applied."""
import json
import subprocess
import time
from pathlib import Path


def command(*args):
    return json.loads(subprocess.check_output(["routeguard", *args]))


before = Path("/etc/routeguard/state.json").read_bytes()
queued = command("queue", "diagnose")
assert queued.get("queued"), queued
for _ in range(900):
    job = command("job", queued["id"])
    if job["state"] != "running":
        break
    time.sleep(0.1)
else:
    raise AssertionError("diagnostics did not finish")
assert job["state"] == "ok", job
checks = job["diagnostics"]["checks"]
wan = [row for row in checks if row.get("device") == "wan"]
assert wan, checks
assert all(row["result"] == "drop_observed" and row["packets"] > 0 for row in wan), checks
assert all(row["result"] != "reachable" for row in checks), checks
assert Path("/etc/routeguard/state.json").read_bytes() == before
assert command("status")["loaded"]
print("PASS: queued WAN-bound IPv4/IPv6 TCP diagnostics observed RouteGuard drops without changing state")
