"""Read the pinned SDK matrix and assemble one complete release."""

import argparse
import hashlib
import json
from pathlib import Path
import shutil


ROOT = Path(__file__).resolve().parent.parent


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("matrix")
    commands.add_parser("sdk").add_argument("architecture")
    commands.add_parser("bundle").add_argument("directory", type=Path)
    args = parser.parse_args()
    manifest = json.loads((ROOT / "scripts/targets.json").read_text())
    targets = manifest["targets"]

    if args.command == "matrix":
        print(json.dumps({"architecture": [item["architecture"] for item in targets]}))
        return

    if args.command == "sdk":
        target = next((item for item in targets if item["architecture"] == args.architecture), None)
        if target is None:
            parser.error(f"Unsupported architecture: {args.architecture}")
        print(
            manifest["version"], target["target"], target["subtarget"],
            target["sdk"], target["sha256"],
            int(args.architecture == manifest["luci_architecture"]),
        )
        return

    expected = {f"routeguard-{item['architecture']}.apk" for item in targets}
    expected.add("luci-app-routeguard.apk")
    actual = {path.name for path in args.directory.glob("*.apk")}
    if actual != expected:
        parser.error(f"Incomplete release: missing {sorted(expected - actual)}, unexpected {sorted(actual - expected)}")

    checksums = []
    for name in sorted(expected):
        package = args.directory / name
        if package.stat().st_size == 0:
            parser.error(f"Empty package: {name}")
        checksums.append(f"{hashlib.sha256(package.read_bytes()).hexdigest()}  {name}\n")

    shutil.copy2(ROOT / "install.sh", args.directory / "install.sh")
    (args.directory / "SHA256SUMS").write_text("".join(checksums))


if __name__ == "__main__":
    main()
