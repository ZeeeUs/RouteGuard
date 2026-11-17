# Configuring RouteGuard

[Русский](CONFIGURATION.md) · English

## Free space

Check the available space before installing:

```sh
df -h /overlay /tmp
```

- **`/overlay` — at least 16 MiB free.** This is recommended headroom for installation, saved rule snapshots, and upgrades when `firewall4`, `ca-bundle`, and `luci-base` are already installed. The measured ARM64 build contains about 6.5 MiB of uncompressed files across the two packages. Actual usage depends on the filesystem and firmware.
- **`/tmp` — at least 16 MiB free.** The installer downloads both APKs here. Across the five local builds, one pair of APKs takes about 2.6–3.0 MiB, with extra space needed for temporary installation files. On most OpenWrt devices, `/tmp` uses RAM.

Allow extra space for any missing dependencies and repository indexes that `apk` needs to download. See [package sizes and memory measurements](RESOURCES.en.md).

## Manual installation and setup

Copy the engine APK matching `/etc/apk/arch` and the common LuCI APK to `/tmp` on the router. Use `--allow-untrusted` for local APKs without a trusted signature only if you control the build. The example uses `aarch64_cortex-a53`.

```sh
cd /tmp
apk update
apk add --allow-untrusted ./routeguard-aarch64_cortex-a53.apk ./luci-app-routeguard.apk
/etc/init.d/rpcd restart
```

Use the filenames from your build. After installing, open **Services → RouteGuard**, signing in to LuCI again if needed. Check the Linux devices used for outgoing traffic over SSH. Verify policy routes and backup WAN devices separately.

```sh
ip -4 route show default
ip -6 route show default
ubus call network.interface.wan status
ip link show
```

Disable software and hardware flow offloading in Firewall and apply the change. Offloaded flows can bypass the nftables hooks. RouteGuard refuses to apply rules if fw4 has offloading enabled or an active nftables flowtable in any family exists.

Create and enable your profiles, choose the WAN devices, and set the IPv6 mode. Enter a DNS server IP for domains and remote lists. If needed, add the tunnel server IP as an exception. Exceptions apply to every profile and every port on that IP. Update the exception if the server's IP changes.

**Save & Apply** saves the RouteGuard configuration and waits for the corresponding operation. If applying rules fails, LuCI shows the error and RouteGuard keeps the last working rules. If only the cron update fails after the rules are installed, the operation completes with a warning. **Refresh active addresses** updates the addresses of the applied configuration. To schedule hourly updates, enter a minute in **Hourly update minute** and enable system cron. Leave the field empty for manual updates.

### UCI example

```uci
config settings 'main'
	option enabled '1'
	option ipv6 'off'
	list wan_device 'pppoe-wan'

config service 'example'
	option enabled '1'
	option label 'My service'
	list ip '192.0.2.10'
```

Use your own WAN device and IP. For domain targets, add `option dns` to settings and `list domain` or `list url` to the profile. After editing through UCI, run `uci commit routeguard` followed by `routeguard apply`. If you edit `/etc/config/routeguard` directly, run `routeguard apply`.

Configuration types and booleans are validated. `enabled` accepts `1/0`, `true/false`, `yes/no`, `on/off`, and `enabled/disabled`. Replace any `option preset` left by an early build with explicit targets.

## Remote domain lists

Paste a direct HTTPS URL into **Settings → profile → Remote HTTPS domain lists**, set **DNS server**, enable the profile, and click **Save & Apply**. On GitHub, use the **Raw** button to get the direct URL.

| Source                                    | Direct link to a plain-text list                                                                          |
| ----------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| v2fly domain-list-community               | [Anthropic](https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/anthropic)          |
| `allow-domains` collection used by Podkop | [Discord](https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/discord.lst) |

You can paste these links directly into the field. They cover different services, so use separate profiles to manage them independently. To combine multiple sources for one service, add them to the same profile using **+**, one URL per row.

- Use plain text with one hostname per line. RouteGuard accepts `#` comments and the `domain:` and `full:` prefixes. It resolves names literally, so list any required subdomains separately. Entries using `include:`, `regexp:`, `keyword:`, or attributes such as `@telemetry` are skipped. For collections that publish multiple formats, choose a plain-text domain list. Binary `geosite.dat`, `.srs` and `.mrs` files and dnsmasq configurations are unsupported.
- A profile takes up to four URLs. Each file can be up to 128 KiB. All enabled profiles together can contain up to 256 unique domains. RouteGuard merges remote lists and manual domains before DNS lookups, ignoring case and trailing dots when removing duplicates. `example.org` and `api.example.org` remain separate names. **Status → Unique domains** shows the count for each profile.
- RouteGuard verifies HTTPS certificates and rejects redirects. If a source is unreachable, empty, or malformed, the previous rules stay active. External lists can change, so review their entries before applying them.

Remote list URLs are restricted to public IP addresses by default. To permit a trusted internal list, root can add its exact URL hostname and an allowed IP prefix to `/etc/routeguard/private-list-hosts`, for example `lists.example.net 10.0.0.2/32`. HTTPS certificate checks also apply to these sources.

### DNS and IPv6

- DNS queries go directly to the configured server. DNS errors, synthetic answers from `198.18.0.0/15`, and profiles without usable addresses leave previous rules active. When a refresh gets `NXDOMAIN` or an empty A/AAAA answer, it retains the last known addresses for that hostname and family for up to 24 hours from the first empty answer. The cache is populated by successful address lookups after upgrading.
- `ipv6=off` leaves IPv6 unfiltered by RouteGuard. `ipv6=selective` enables AAAA lookups and manual IPv6 addresses or CIDRs. Add both outgoing devices if your IPv4 and IPv6 routes use different ones.
- Clients may get different IPs because of CDNs, geography, resolver choice, or query time. Unlisted IPs and routes through unselected WAN devices can pass. A shared CDN IP can affect other services. RouteGuard filters traffic that goes through this router.

## Status, rollback, and removal

```sh
routeguard status
logread -e routeguard
nft list table inet routeguard
routeguard rollback
```

Rollback restores the previous active snapshot and pauses scheduled refreshes. `routeguard apply` applies current UCI settings and resumes the schedule if an update minute is set. RouteGuard checks the chains, priorities, rule order, WAN selectors, and live address sets before reporting unchanged rules. It repairs missing or damaged rules. Counters survive unchanged rules and reset when RouteGuard replaces the table or the router reboots.

To remove the rules, set `routeguard.main.enabled=0`, save the configuration, and run `routeguard apply`. To uninstall, stop the service before removing the packages. UCI settings and snapshots can be retained for reinstallation. Reinstalling with an active snapshot restores it at startup. Disable and apply the configuration before uninstalling if you want the next installation to start with protection disabled.

```sh
/etc/init.d/routeguard stop
apk del luci-app-routeguard routeguard
nft list table inet routeguard
```

The last command should report that the table is missing. To remove saved state, delete `/etc/routeguard/state.json` and `/etc/routeguard/state.json.pending` if it exists, after stopping the service. RouteGuard manages its own nftables table and its own line in the root crontab. Standard `fw4 reload` and `fw4 restart` use `inet fw4`. A global `nft flush ruleset` also removes RouteGuard rules. Use **Status → Restore saved rules** or `routeguard restore` to reinstall the local snapshot. If the table disappears, refresh restores the snapshot before contacting address sources. See the [fw4 command implementation](https://lxr.openwrt.org/source/firewall4/root/sbin/fw4).

## LuCI profiles and status

Enter a name in **New profile name** and click **Create profile**. The name becomes the section heading. Enable **Protect this profile** when the profile is ready. The global **Block direct connections** switch controls whether protection is applied.

Select one or more **Internet uplink devices** such as `wan` or `pppoe-wan`. LuCI retains saved devices that are temporarily absent and allows manual entry. Choose the external uplinks. In **DNS server**, choose Cloudflare, Google, or Quad9, or enter your own IPv4 or IPv6 address. The field starts empty and can stay empty for IP-only profiles.

The **Status** tab shows live protection, the schedule, the last operation, active profiles, and separate drop counters for LAN clients and the router. It reflects applied settings and displays times in the browser's time zone. `routeguard status` returns indented JSON for scripts and RPC.

## Diagnostics

**Save & Apply** applies the global settings and profiles together. In **Diagnostics**, **Check all profiles** attempts TCP connections from the router to port 443 on sampled addresses, binding each connection to a selected WAN device. It uses TCP directly and reads the firewall counters. This requires verified active rules, matching saved and applied settings, disabled flow offloading, and no pending recovery. Other RouteGuard operations wait or return a busy error during the check.

When refreshing addresses, RouteGuard saves up to one IPv4 and one IPv6 test address per profile. It skips exempt addresses and CIDR ranges without individual host targets. After upgrading from an older release, refresh addresses on **Status** before using diagnostics. Disabled profiles and profiles without a suitable address show **Not tested**.

- **Connection succeeded** means the probe connected. Local traffic redirection can affect its path.
- **Drop observed** means the probe failed and the OUTPUT drop counter increased. The counter can include background traffic.
- **Inconclusive** includes timeouts with no counter increase.

These probes sample outbound TCP on the router. LAN forwarding, UDP, other ports and addresses, and tunnel availability require separate checks. A run uses at most 32 probes and 90 seconds, with up to two seconds per connection. Results are stored by operation ID in `/var/run/routeguard/jobs`. To start a check from the CLI, run `routeguard queue diagnose` and read the report with `routeguard job ID`.
