# RouteGuard

[Русский](README.md) · English

A selective WAN kill switch for OpenWrt. RouteGuard blocks direct connections to known IP addresses of selected services on the WAN devices you choose.

<p align="center">
  <img src="assets/routeguard-lynx-mascot.png" alt="RouteGuard lynx mascot" width="340">
</p>

## How it works

- **LuCI and UCI.** Configure profiles, WAN devices, and exceptions in LuCI. Settings are saved in UCI.
- **Go engine.** Collects IP addresses from manual entries, domains, and remote lists, applies firewall rules, and saves a snapshot for recovery and rollback.
- **nftables and cron.** The kernel filters traffic. System cron refreshes addresses on the configured schedule.

## Installation

Packages target official OpenWrt 25.12. The installer selects the APK for your package architecture.

Run as root on the router:

```sh
sh <(wget -O - https://raw.githubusercontent.com/ZeeeUs/RouteGuard/refs/heads/main/install.sh)
```

The installer downloads prebuilt APKs from GitHub Releases, verifies their checksums, and installs them.

With the dependencies already installed, allow **at least 16 MiB free in `/overlay`** and **at least 16 MiB free in `/tmp`**. Check both with `df -h /overlay /tmp`. See [package sizes and dependencies](docs/CONFIGURATION.en.md#free-space).

## Compatibility

| Package architecture | Devices and platforms                          | Tested on hardware |
| -------------------- | ---------------------------------------------- | ------------------ |
| `aarch64_cortex-a53` | MediaTek Filogic (`mediatek/filogic`)          | ✅                 |
| `aarch64_generic`    | NanoPi R2S / R4S / R5S / R6S, `rockchip/armv8` | ❌                 |
| `aarch64_cortex-a72` | Raspberry Pi 4 / CM4, `bcm27xx/bcm2711`        | ❌                 |
| `aarch64_cortex-a76` | Raspberry Pi 5, `bcm27xx/bcm2712`              | ❌                 |
| `x86_64`             | Mini PCs and virtual machines, `x86/64`        | ❌                 |

The release build covers all five architectures. Hardware testing on MediaTek MT7981B with OpenWrt 25.12.5 verified installation, blocking in the OUTPUT and FORWARD chains, and protection after reboot and `fw4 reload`. Other devices with the same architecture, FriendlyWrt, ImmortalWrt, and vendor firmware still need testing.

## Getting started

1. Disable software and hardware **flow offloading** in the Firewall settings.
2. Open **Services → RouteGuard**. Select your WAN devices and the IPv6 mode.
3. Add and enable a profile with IP addresses, domains, or a list URL. Set a DNS server for domains and lists.
4. Enable RouteGuard and click **Save & Apply**. Check active rules and counters on the **Status** tab.

RouteGuard starts with an empty configuration. Filtering starts when you apply your settings. Addresses outside the collected set can pass. IPv6 Off leaves only IPv4 filtering active.

Profiles can use plain-text lists over HTTPS. See [examples from v2fly and the list collection used by Podkop](docs/CONFIGURATION.en.md#remote-domain-lists).

[Configuration, rollback, and removal](docs/CONFIGURATION.en.md)
