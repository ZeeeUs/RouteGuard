"""Measure repeatable Linux workloads and enforce development resource budgets."""

import importlib.util
import json
import pathlib
import platform
import subprocess
import time
from datetime import datetime, timezone

ROOT = pathlib.Path("/src")
OUT = pathlib.Path("/out")
spec = importlib.util.spec_from_file_location("fixture", ROOT / "tests/integration/resource-server.py")
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)


def prepare(maximum):
    pathlib.Path("/tmp/resource-mode").write_text("maximum" if maximum else "typical")
    domains = 256 if maximum else 16
    records = "".join(f"x{index:04d}.example.org\n" for index in range(domains))
    if maximum:
        records = (records + ("#" + "x" * 252 + "\n") * 600)[: 128 * 1024]
    pathlib.Path("/tmp/resource-list").write_text(records)

    lines = ["config settings 'main'", "option enabled '1'", "option ipv6 'selective'", "option dns '127.0.0.1'"]
    for device in range(32 if maximum else 1):
        lines.append(f"list wan_device 'wan{device}'")
    if maximum:
        for index in range(128):
            lines.append(f"list exempt_ip '{fixture.address(index, True)}'")
    for service in range(8 if maximum else 1):
        lines += [f"config service 'profile{service}'", "option enabled '1'"]
        for source in range(4 if maximum else 1):
            lines.append(f"list url 'https://127.0.0.1/list?source={source}'")
        if maximum:
            for index in range(service * 512, (service + 1) * 512):
                lines.append(f"list ip '{fixture.address(index, True)}'")
    pathlib.Path("/etc/config/routeguard").write_text("\n".join(lines) + "\n")


def measure(name, command):
    rss_file = OUT / f"{name}.rss"
    start = time.monotonic()
    subprocess.run(["/usr/bin/time", "-f", "%M", "-o", str(rss_file), *command], check=True)
    return {"peak_process_rss_kib": int(rss_file.read_text()), "seconds": round(time.monotonic() - start, 3)}


budgets = json.loads((ROOT / "tests/resource-budgets.json").read_text())
report = {
    "measured_utc": datetime.now(timezone.utc).isoformat(),
    "minimum_go": (ROOT / "routeguard/src/go.mod").read_text().split("go ")[1].strip(),
    "platform": platform.platform(),
    "architecture": platform.machine(),
    "nft": subprocess.check_output(["nft", "--version"], text=True).strip(),
    "binary_bytes": (ROOT / "build/routeguard").stat().st_size,
    "rss_method": "GNU time %M: largest process RSS reported for the command, including children; not simultaneous process-tree RSS or kernel memory",
    "workloads": {},
}
payload = [(ROOT / "build/routeguard")] + list((ROOT / "luci-app-routeguard/htdocs").rglob("*")) + list((ROOT / "luci-app-routeguard/root").rglob("*")) + list((ROOT / "routeguard/files").glob("*"))
report["installed_payload_bytes"] = sum(path.stat().st_size for path in payload if path.is_file())

for maximum in (False, True):
    name = "maximum" if maximum else "typical"
    prepare(maximum)
    samples = [measure(f"{name}-{index}", ["routeguard", "apply"]) for index in range(3)]
    result = {
        "peak_process_rss_kib": max(sample["peak_process_rss_kib"] for sample in samples),
        "samples": samples,
    }
    status = json.loads(subprocess.check_output(["routeguard", "status"]))
    assert status["loaded"], status
    result["domains"] = 256 if maximum else 16
    result["targets"] = 4096 if maximum else 64
    state = json.loads(pathlib.Path("/etc/routeguard/state.json").read_text())
    assert len(state["active"]["plan"]["targets"]) == result["targets"], state["active"]["services"]
    result["services"] = len(status["services"])
    result["status"] = measure(name + "-status", ["routeguard", "status"])
    nft_text = subprocess.check_output(["nft", "list", "table", "inet", "routeguard"], text=True)
    batch = OUT / "measurement.nft"
    batch.write_text("destroy table inet routeguard\n" + nft_text)
    result["nft_validate"] = measure(name + "-nft-validate", ["nft", "-c", "-f", str(batch)])
    result["nft_apply"] = measure(name + "-nft-apply", ["nft", "-f", str(batch)])
    result["state_bytes"] = pathlib.Path("/etc/routeguard/state.json").stat().st_size
    report["workloads"][name] = result

(OUT / "resources.json").write_text(json.dumps(report, indent=2) + "\n")
assert report["binary_bytes"] <= budgets["binary_bytes"], report
assert report["installed_payload_bytes"] <= budgets["installed_payload_bytes"], report
for result in report["workloads"].values():
    assert result["peak_process_rss_kib"] <= budgets["command_peak_rss_kib"], result
    assert result["status"]["peak_process_rss_kib"] <= budgets["command_peak_rss_kib"], result
    assert result["nft_validate"]["peak_process_rss_kib"] <= budgets["nft_peak_rss_kib"], result
    assert result["nft_apply"]["peak_process_rss_kib"] <= budgets["nft_peak_rss_kib"], result
print(json.dumps(report, indent=2))
