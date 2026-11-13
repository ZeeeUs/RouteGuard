"""Verify APK metadata, required files and the engine's ELF architecture."""

import argparse
import json
from pathlib import Path
import re
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("apk_tool", type=Path)
    parser.add_argument("package", type=Path)
    parser.add_argument("architecture")
    args = parser.parse_args()
    package = args.package.resolve()
    apk = str(args.apk_tool.resolve())
    metadata = subprocess.check_output([apk, "adbdump", str(package)], text=True)
    architecture = re.search(r"^  arch: (.+)$", metadata, re.MULTILINE)
    if architecture is None or architecture.group(1) != args.architecture:
        parser.error(f"Unexpected APK architecture in {package.name}")

    with tempfile.TemporaryDirectory(prefix="routeguard-apk-") as temporary:
        subprocess.run([apk, "extract", "--allow-untrusted", "--no-chown",
                        "--destination", temporary, str(package)], check=True)
        root = Path(temporary)
        if args.architecture == "noarch":
            required = [
                "usr/libexec/rpcd/luci.routeguard",
                "usr/share/rpcd/acl.d/luci-app-routeguard.json",
                "usr/share/luci/menu.d/luci-app-routeguard.json",
                "www/luci-static/resources/view/routeguard/overview.js",
            ]
        else:
            required = [
                "usr/sbin/routeguard", "etc/config/routeguard", "etc/init.d/routeguard",
                "etc/hotplug.d/iface/95-routeguard", "lib/upgrade/keep.d/routeguard",
            ]
        for relative in required:
            if not (root / relative).is_file():
                parser.error(f"Missing {relative} in {package.name}")

        if args.architecture != "noarch":
            binary = root / "usr/sbin/routeguard"
            with binary.open("rb") as stream:
                header = stream.read(20)
            machine = 62 if args.architecture == "x86_64" else 183
            if header[:6] != b"\x7fELF\x02\x01" or int.from_bytes(header[18:20], "little") != machine:
                parser.error(f"Wrong ELF architecture in {package.name}")
            budgets = json.loads((Path(__file__).resolve().parent.parent / "tests/resource-budgets.json").read_text())
            if binary.stat().st_size > budgets["binary_bytes"]:
                parser.error(f"Engine binary exceeds size budget in {package.name}")

    print(f"Verified {package.name}: {args.architecture}, metadata and contents")


if __name__ == "__main__":
    main()
