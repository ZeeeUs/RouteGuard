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


def router_environment(bin_directory, compatibility_only=False):
    fixture = ROOT / "tests/release/fixtures/router.sh"
    tools = ["id", "ubus", "cat", "jsonfilter", "apk", "wget"]
    if compatibility_only:
        tools.append("usign")
    for tool in tools:
        destination = bin_directory / tool
        shutil.copyfile(fixture, destination)
        destination.chmod(0o755)
    return dict(os.environ, PATH=f"{bin_directory}:{os.environ['PATH']}",
                TEST_DISTRIBUTION="OpenWrt", TEST_VERSION="25.12.5", TEST_TARGET="different/target",
                TEST_ARCH=ARCHITECTURES[0])


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
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            router_bin = root / "bin"
            router_bin.mkdir()
            assets = root / "assets"
            assets.mkdir()
            public = root / "release.pub"
            public.touch()
            log = root / "requests"
            environment = router_environment(router_bin, compatibility_only=True)
            environment.update(
                TEST_TAG="v1.2.3", TEST_ASSETS=str(assets), TEST_LOG=str(log),
                ROUTEGUARD_RELEASE_KEY=str(public),
            )

            def run_installer(**overrides):
                log.write_text("")
                return subprocess.run(
                    ["sh", str(ROOT / "install.sh")], env=environment | overrides,
                    capture_output=True, text=True,
                )

            for architecture in ARCHITECTURES:
                with self.subTest(architecture=architecture):
                    # The fixture has no manifest. Reaching its first download
                    # proves the firmware and architecture checks passed.
                    result = run_installer(TEST_ARCH=architecture)
                    self.assertIn(f"packages for {architecture}", result.stdout)
                    self.assertIn("could not download SHA256SUMS", result.stderr)
                    self.assertEqual(log.read_text().splitlines(), ["SHA256SUMS"])

            for change in [{"TEST_ARCH": "mipsel_24kc"}, {"TEST_VERSION": "24.10.4"}, {"TEST_DISTRIBUTION": "FriendlyWrt"}]:
                with self.subTest(**change):
                    result = run_installer(**change)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn("unsupported firmware" if "TEST_ARCH" not in change else "no release build", result.stderr)
                    self.assertEqual(log.read_text(), "")

    def test_installer_checks_a_real_signature_before_allow_untrusted(self):
        usign = shutil.which("usign")
        if not usign:
            self.skipTest("usign is required for a real OpenWrt signature")

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            bin_directory, assets = root / "bin", root / "assets"
            bin_directory.mkdir()
            assets.mkdir()
            env = router_environment(bin_directory)
            public, private = root / "release.pub", root / "key.pem"
            log = root / "requests"
            env.update(TEST_TAG="v1.2.3", TEST_ASSETS=str(assets), TEST_LOG=str(log),
                       ROUTEGUARD_RELEASE_KEY=str(public))
            engine = f"routeguard-{env['TEST_ARCH']}.apk"
            names = [f"routeguard-{arch}.apk" for arch in ARCHITECTURES] + ["luci-app-routeguard.apk"]
            for name in names:
                (assets / name).write_bytes(name.encode())
            bundle = release("bundle", assets, "--tag", env["TEST_TAG"])
            self.assertEqual(bundle.returncode, 0, bundle.stderr)
            manifest, signature = assets / "SHA256SUMS", assets / "SHA256SUMS.sig"

            def key_command(*args):
                subprocess.run([usign, *map(str, args)], check=True, capture_output=True)

            key_command("-G", "-s", private, "-p", public)

            def sign():
                key_command("-S", "-s", private, "-m", manifest, "-x", signature)

            def install(*args, script=ROOT / "install.sh", environment=None):
                log.write_text("")
                result = subprocess.run(
                    ["sh", str(script), *args], env=environment or env, capture_output=True, text=True,
                )
                return result, log.read_text().splitlines()

            signed = manifest.read_bytes()
            sign()
            result, requests = install(env["TEST_TAG"])
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            installed = [
                "SHA256SUMS", "SHA256SUMS.sig", engine, "luci-app-routeguard.apk",
                "apk update", f"apk add --allow-untrusted ./{engine} ./luci-app-routeguard.apk",
            ]
            self.assertEqual(requests, installed)

            rendered = release("embed-key", assets, "--signing-key", private, "--usign", usign)
            self.assertEqual(rendered.returncode, 0, rendered.stderr)
            generated = (assets / "install.sh").read_text()
            payload = public.read_text().splitlines()[1]
            self.assertIn(f"embedded_public_key='{payload}'", generated)
            self.assertNotIn(private.read_text().splitlines()[1], generated)

            # Simulate a router without a configured key independently of the host's /etc.
            default_key = root / "router.release.pub"
            script = root / "install.sh"
            script.write_text(generated.replace("/etc/routeguard/release.pub", str(default_key)))
            automatic = env | {"ROUTEGUARD_RELEASE_KEY": ""}
            result, requests = install(env["TEST_TAG"], script=script, environment=automatic)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual(requests, installed)

            other_private = root / "other.sec"
            key_command("-G", "-s", other_private, "-p", default_key)
            wrong = release("embed-key", assets, "--signing-key", other_private, "--usign", usign)
            self.assertNotEqual(wrong.returncode, 0)
            self.assertIn("did not verify", wrong.stderr)
            self.assertEqual((assets / "install.sh").read_text(), generated)

            result, requests = install(script=script, environment=automatic)
            self.assertIn("did not match", result.stderr)
            self.assertEqual(requests, ["SHA256SUMS", "SHA256SUMS.sig"])

            result, requests = install(
                script=script, environment=env | {"ROUTEGUARD_RELEASE_KEY": str(root / "missing.pub")}
            )
            self.assertIn("cannot read the configured", result.stderr)
            self.assertFalse(requests)

            # An explicit, valid key takes precedence over the default local key.
            result, requests = install(script=script)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual(requests, installed)

            manifest.write_bytes(signed + b"# changed\n")
            result, requests = install()
            self.assertIn("did not match", result.stderr)
            self.assertEqual(requests, ["SHA256SUMS", "SHA256SUMS.sig"])

            manifest.write_bytes(signed.replace(b"v1.2.3", b"v1.1.0", 1))
            sign()
            result, requests = install()
            self.assertIn("does not match release", result.stderr)
            self.assertEqual(requests, ["SHA256SUMS", "SHA256SUMS.sig"])

            manifest.write_bytes(signed)
            sign()
            (assets / engine).write_bytes(b"changed")
            result, requests = install()
            self.assertIn("checksum mismatch", result.stderr)
            self.assertEqual(requests, ["SHA256SUMS", "SHA256SUMS.sig", engine])

            public.unlink()
            result, requests = install()
            self.assertIn("cannot read the configured", result.stderr)
            self.assertFalse(requests)


if __name__ == "__main__":
    unittest.main()
