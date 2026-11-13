"""Check release completeness and installer selection without touching a router."""

import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
MANIFEST = json.loads((ROOT / "scripts/targets.json").read_text())
ARCHITECTURES = [target["architecture"] for target in MANIFEST["targets"]]


def release(*args):
    return subprocess.run(
        [sys.executable, str(ROOT / "scripts/release.py"), *map(str, args)],
        capture_output=True, text=True,
    )


class ReleaseTests(unittest.TestCase):
    def test_matrix_and_sdk_selection(self):
        self.assertEqual(len(set(ARCHITECTURES)), 5)
        self.assertEqual(json.loads(release("matrix").stdout)["architecture"], ARCHITECTURES)
        for target in MANIFEST["targets"]:
            with self.subTest(architecture=target["architecture"]):
                selected = release("sdk", target["architecture"])
                self.assertEqual(selected.returncode, 0, selected.stderr)
                fields = selected.stdout.split()
                self.assertEqual(fields[:3], [MANIFEST["version"], target["target"], target["subtarget"]])
                self.assertEqual(fields[3:5], [target["sdk"], target["sha256"]])
                self.assertRegex(target["sha256"], r"^[a-f0-9]{64}$")
                self.assertEqual(fields[5], str(int(target["architecture"] == MANIFEST["luci_architecture"])))
        self.assertNotEqual(release("sdk", "unsupported").returncode, 0)

    def test_bundle_requires_every_package_and_checksums_actual_content(self):
        with tempfile.TemporaryDirectory() as directory:
            dist = Path(directory)
            names = [f"routeguard-{arch}.apk" for arch in ARCHITECTURES] + ["luci-app-routeguard.apk"]
            for name in names:
                self.assertNotEqual(release("bundle", dist).returncode, 0)
                (dist / name).write_bytes(name.encode())

            result = release("bundle", dist)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((dist / "install.sh").read_bytes(), (ROOT / "install.sh").read_bytes())
            for line in (dist / "SHA256SUMS").read_text().splitlines():
                digest, name = line.split()
                self.assertEqual(digest, hashlib.sha256((dist / name).read_bytes()).hexdigest())

            unexpected = dist / "unexpected.apk"
            unexpected.write_bytes(b"unexpected")
            self.assertNotEqual(release("bundle", dist).returncode, 0)
            unexpected.unlink()
            (dist / names[0]).write_bytes(b"")
            self.assertNotEqual(release("bundle", dist).returncode, 0)

    def test_installer_accepts_matrix_architectures_across_targets(self):
        # Execute the real compatibility checks, stopping before any downloads.
        checks = (ROOT / "install.sh").read_text().split("umask 077", 1)[0]
        with tempfile.TemporaryDirectory() as directory:
            bin_directory = Path(directory)
            fixture = ROOT / "tests/release/fixtures/router.sh"
            for tool in ["id", "ubus", "cat", "jsonfilter", "apk", "sha256sum", "wget"]:
                destination = bin_directory / tool
                shutil.copyfile(fixture, destination)
                destination.chmod(0o755)

            environment = dict(os.environ, PATH=f"{bin_directory}:{os.environ['PATH']}",
                               TEST_DISTRIBUTION="OpenWrt", TEST_VERSION="25.12.5", TEST_TARGET="different/target")
            for architecture in ARCHITECTURES:
                environment["TEST_ARCH"] = architecture
                result = subprocess.run(["sh", "-c", checks], env=environment, capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            for change in [{"TEST_ARCH": "mipsel_24kc"}, {"TEST_VERSION": "24.10.4"}, {"TEST_DISTRIBUTION": "FriendlyWrt"}]:
                result = subprocess.run(["sh", "-c", checks], env=environment | change, capture_output=True, text=True)
                self.assertNotEqual(result.returncode, 0)


if __name__ == "__main__":
    unittest.main()
