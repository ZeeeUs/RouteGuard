# Size and memory

[Русский](RESOURCES.md) · English

See the [installation guide](CONFIGURATION.en.md#free-space) for recommended free space and how to check it with `df`.

Measurements were taken on November 11, 2026 with Go 1.27.4, Linux ARM64, and nftables 1.1.3. See the [full results and test environment](results/linux-arm64.json).

| Metric                                   | Typical configuration | Largest test configuration |
| ---------------------------------------- | --------------------: | -------------------------: |
| Profiles / domains / addresses           |      1 / 16 / 64 IPv4 |        8 / 256 / 4096 IPv6 |
| Peak RSS on apply, highest of three runs |             15.55 MiB |                  17.75 MiB |
| Peak RSS of the status command           |             10.03 MiB |                  16.35 MiB |
| Peak RSS of a separate `nft -f` command  |              4.43 MiB |                  10.30 MiB |

The Linux ARM64 binary is **6,815,904 bytes (6.50 MiB)**. Before packaging, the uncompressed files of the two packages total about **6.51 MiB**, excluding system dependencies. The SDK build checks APK size separately. Actual usage in `/overlay` depends on the filesystem and installed dependencies.

The measured SDK build produced an engine APK of **2,759,087 bytes (2.63 MiB)** and a LuCI APK of **4,800 bytes**. The SDK processed binary occupies 6,797,464 bytes and the files in both APKs total 6,807,604 bytes (6.49 MiB). See the [package checksums and installation results](results/openwrt-25.12.5.json).

## Measurement procedure

```sh
make resources TARGET_ARCH=arm64
```

The script runs in a separate Docker container with no external network access. It provides local DNS and HTTPS services with a test certificate. The typical profile gets 64 IPv4 addresses for 16 domains. The largest configuration uses 32 WAN devices, 8 profiles, 256 domains, 4,096 unique IPv6 addresses, 128 exceptions, 512 manual addresses, and four URLs per profile. Each remote source reaches the 128 KiB limit.

The three apply runs include the first change and subsequent checks of the installed table. The script measures status, `nft` validation, and rule loading separately. It saves results to `build/resources.json` and fails if a budget is exceeded. For x86_64, use `TARGET_ARCH=amd64`.

GNU `time %M` reports the highest RSS for the command, including child processes. It measures peak RSS for a process. To measure the combined memory of concurrently running processes or the memory used for kernel tables, use separate measurements. Results depend on GC behavior, architecture, and kernel. A larger profile can have a lower peak RSS in an individual run.

## Regression budgets

The thresholds are defined in [`tests/resource-budgets.json`](../tests/resource-budgets.json):

- Binary: up to 8 MiB.
- Uncompressed package files, excluding dependencies: up to 9 MiB.
- Command peak RSS: up to 32 MiB.
- Separate `nft` process: up to 16 MiB.
- Engine APK: up to 4 MiB.

These budgets flag regressions. Go's soft 12 MiB memory target controls GC behavior. Process RSS can exceed it. RouteGuard runs processes on demand. LuCI checks status every 10 seconds while the page is open. Results from the last 16 operations are kept in `/var/run`.

To measure flash and overall RAM usage, including kernel tables, follow the [hardware test checklist](../OPENWRT-TEST.en.md#size-and-memory) on the target router.
