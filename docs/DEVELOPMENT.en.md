# RouteGuard development

[Русский](DEVELOPMENT.md) · English

## Components

| Directory in `routeguard/src` | Responsibility                                                                |
| ----------------------------- | ----------------------------------------------------------------------------- |
| `cmd/routeguard`              | CLI, dependency wiring, and Go runtime memory target                          |
| `internal/config`             | UCI parsing into typed structures, profile IDs, and strict boolean parsing    |
| `internal/sources`            | Reading bounded HTTPS lists, DNS queries, and rejecting synthetic DNS answers |
| `internal/policy`             | Settings validation and deterministic address plans without system changes    |
| `internal/nft`                | Rule rendering and validation of nftables JSON, including merged intervals    |
| `internal/platform`           | External commands, devices, routes, and cron                                  |
| `internal/store`              | State format versioning, migration, bounded reads, and atomic, durable writes |
| `internal/app`                | Apply, recovery, rollback, locking, and operation results                     |

The CLI wires the concrete adapters. Tests supply dependencies through small interfaces and avoid global mutable paths or `PATH` overrides in Go. Organize packages by responsibility.

LuCI separates form fields, status, operations, and views. **Save & Apply** saves only RouteGuard settings and waits for its own job. Refresh works from the last applied configuration. Rollback pauses scheduled refreshes.

## Invariants

- A fresh installation has no profiles, WAN devices, DNS server, IPv6 mode, or schedule configured.
- Configuration, download, DNS, or nftables validation errors preserve the active protection.
- Verification checks nftables hooks, priorities, policies, rules, WAN selectors, and addresses.
- One nftables transaction updates only `inet routeguard`. Unchanged rules retain their counters.
- The intended state is written and synced before updating kernel rules. Recovery replays it until state is consistent. Rollback records its own intent before updating the kernel.
- Damaged or unsupported state files are retained.
- A background operation is accepted once its worker confirms it has started. The lock survives handoff, and the kernel releases it when the process exits.

## Checks

`make check` runs Go tests, `go vet`, formatting and shell checks, LuCI tests for save and apply, and release checks. Text fixtures live in `testdata` and use `go:embed`. Small scalar values and generated address sets stay in the test code.

Unit tests use [Uber GoMock](https://github.com/uber-go/mock) for interfaces. `go.mod` pins the library and `mockgen`, which runs through `go tool`. After changing an interface, run `make generate` and commit the generated `mock/*_mock.go` files. CI regenerates the mocks and compares the result with the checked-in files.

Set expectations in the test that uses them. Specify the arguments and call counts. Use `gomock.InOrder` when order affects correctness. Reserve `AnyTimes` for incidental logging. Recovery tests use the real store in a temporary directory and inject errors at relevant operation boundaries. DNS, process handoff, and nftables integration tests exercise the real operations.

Mocks live in `mock` directories next to their interface definitions, with the original filename followed by `_mock.go`. The mock for `net/http.RoundTripper` lives at `internal/sources/mock/round_tripper_mock.go`. Generation directives are in `generate_test.go`. GoMock and its generator are development dependencies. Router builds of `./cmd/routeguard` can still use `GOPROXY=off`.

`make integration TARGET_ARCH=arm64` runs in a separate container with network namespaces for LAN and two WANs. It tests packet forwarding, IPv4 and IPv6, both hooks, exceptions, idempotence, table damage, failed apply, rollback, and recovery. The container uses privileges to create the test networks and runs without host network access.

`make resources TARGET_ARCH=arm64` runs local DNS and HTTPS inside a container. The largest test uses eight profiles, 32 WAN devices, 256 domains, 4,096 addresses, 128 exceptions, and four sources per profile. All source files are generated locally. `tests/resource-budgets.json` sets the thresholds. Changes to those thresholds require an explanation and fresh measurements.

For x86_64, use `TARGET_ARCH=amd64`. Use the [hardware checklist](../OPENWRT-TEST.en.md) to verify target OpenWrt behavior and LuCI in a browser.

## Release builds

`go.mod` pins Go, `package-lock.json` pins formatting tools, and [`scripts/targets.json`](../scripts/targets.json) pins the target architectures, SDKs, and SHA-256 checksums. The manifest supplies the GitHub Actions matrix. The OpenWrt recipe uses Go from `PATH` and builds without fetching modules.

```sh
sh scripts/build-sdk.sh /path/to/sdk /path/to/dist
```

The optional fourth argument selects the package architecture. For example, run `sh scripts/build-sdk.sh /path/to/sdk-x86 /path/to/dist-x86 v0.1.0 x86_64`. The default is `aarch64_cortex-a53`. Use a separate SDK directory per architecture. The SDK marker records its version, architecture, and SHA-256. If your directory has the older empty marker, use a fresh directory.

The `luci_architecture` job also builds the common LuCI APK. After all jobs succeed, `scripts/release.py bundle` checks for exactly five engine APKs and one LuCI APK and writes their combined `SHA256SUMS`. Publication requires a complete build. A manual workflow run produces the `openwrt-release` artifact for testing.

CI runs unit, UI, network, and resource checks before releasing. SDK builds require Linux x86_64 and the dependencies in `tests/integration/Dockerfile.sdk`. The build script also checks the engine package size.

A `vMAJOR.MINOR.PATCH` Git tag sets the version for both packages. For example, `v0.0.2` produces `0.0.2-r1`. Local builds and workflow runs from a branch without a tag use `0.0.0-r1`. To build a release version locally, pass its tag as the third argument.

The Go engine is statically linked, and LuCI contains JavaScript and shell assets. The script builds the LuCI host tools, then uses `NO_DEPS=1` for RouteGuard's own packages. The APK dependency declarations still tell `apk` to install firewall4, certificates, and LuCI from the corresponding OpenWrt repositories. The build needs no firmware or kernel module rebuild.

### Release signing

Generate a dedicated `usign` key pair on a trusted machine. Put the contents of the secret key file in the GitHub Actions secret `ROUTEGUARD_SIGNING_KEY`. Keep the private key outside the repository.

```sh
umask 077
usign -G -s routeguard-release.sec -p routeguard-release.pub
```

For a tagged release, the workflow signs `SHA256SUMS`, extracts the public part of the key, and embeds it in the published `install.sh` after verifying the signature. The `SHA256SUMS.sig` signature covers the manifest containing the release tag and APK checksums. Share the SHA-256 of the published `install.sh` over a trusted channel so users can verify it before running it.

Set `ROUTEGUARD_RELEASE_KEY` to the path of an explicitly trusted public key. A key at `/etc/routeguard/release.pub` also takes precedence over the embedded key. For the installer from source or a test artifact, configure one of these keys manually.

## Testing built APKs

`scripts/check-apk.py` verifies APK architecture, required files, and the engine ELF architecture on each SDK build. `tests/openwrt/run.sh` installs and tests the packages in a container with an official OpenWrt 25.12 root filesystem. It reads `/etc/apk/arch` to select the engine package. The [OpenWrt 25.12.5 test report](results/openwrt-25.12.5.json) records the Filogic run, image checksum, and package sizes.

To prepare the container, extract `sysupgrade-glinet_gl-mt6000/root` from the firmware archive in the report. Unpack the SquashFS with `unsquashfs`, then import a tar of its contents with `docker import --platform linux/arm64 - routeguard-openwrt:local`. The SDK provides `staging_dir/host/bin/unsquashfs4`. Extract in a separate container when the filesystem contains device nodes.

On Linux ARM64 or Docker with ARM64 support:

```sh
docker run --rm --privileged --network=none \
  -v "$PWD:/src:ro" -v "$PWD/build/dist:/packages:ro" -v "$PWD/build:/out" \
  routeguard-openwrt:local sh /src/tests/openwrt/run.sh
```

The container tests installation and removal, empty default configuration, UCI, nftables, RPC, and the sysupgrade keep list. It uses the Docker kernel. Verify boot through procd, LuCI in a browser, and physical router behavior using the [hardware checklist](../OPENWRT-TEST.en.md).
