# Hardware tests on OpenWrt 25.12.5 / mediatek/filogic

[Русский](OPENWRT-TEST.md) · English

1. Back up `/etc/config/firewall` and root's crontab. Compare `ubus call system board` and `/etc/apk/arch` with the firmware version and package architecture. `apk --print-arch` may show the generic compiler architecture such as `aarch64`. Install both APKs using the release installer or [install them manually](docs/CONFIGURATION.en.md#manual-installation-and-setup). Open **Services → RouteGuard**. Check that the form is empty and that there is no RouteGuard table or cron job.
2. Disable flow offloading. Choose the actual WAN devices and IPv6 mode, enable RouteGuard, and create a test profile. Use the IP of an external HTTPS server you control. Verify that the server is reachable directly before applying the profile. Add the IP as a manual address and click **Save & Apply** once. Wait for the operation to finish, then check `routeguard status` for `loaded=true` and the profile. Verify that invalid form input prevents apply and that LuCI shows engine errors.
3. Test a direct connection to that IP from both the router and a LAN client, with traffic interception disabled for the test address. The connections should fail and the `output4` and `forward4` counters should increase independently. Repeat with IPv6 in selective mode and check `output6` and `forward6`. In off mode, RouteGuard should remove its IPv6 rules. Repeat with a backup WAN. A separate control IP should remain reachable.
4. Set up a profile with a domain you control and a DNS server. Verify access through the tunnel and blocking when the client connects directly. Check the RouteGuard counters and confirm that the router can still reach the tunnel endpoint. To test exceptions, add one test IP to both the block list and exceptions, then confirm that the direct connection succeeds.
5. If another firewall rule covers the same addresses, also test a control IP that the other rule allows. After `fw4 reload`, the RouteGuard table and its counters should remain intact.
6. Test a failed update by adding an unreachable URL and applying the new UCI configuration. The status should report an error and keep the old table and counters. Correct the URL and apply again. To test rollback, apply two different snapshots and roll back the second. Cron should leave the restored snapshot in place until the next explicit apply.
7. Set an update minute and apply, run `fw4 reload`, and reboot the router. A normal reboot should restore the table from the saved snapshot without downloading lists or resolving domains. The counters should restart at zero. Check root's crontab and the log after the scheduled minute. Clear the update minute, apply, and confirm that the cron job is removed.
8. Follow the [removal instructions](docs/CONFIGURATION.en.md#status-rollback-and-removal). Confirm that RouteGuard's cron entry and `inet routeguard` table are gone, other cron entries remain, `inet fw4` remains active, and you can still access the router.

Additional recovery checks for a test router:

- After a successful apply, clear the rules in `inet routeguard forward`. Status should report a mismatch. `routeguard refresh` should repair the chain. Verify blocking by making a connection. Repeat after clearing one address set.
- Run `routeguard queue apply` and read the result with `routeguard job ID`. If you terminate that worker, the operation should become `interrupted` and LuCI should allow a retry. Limit the interruption to the RouteGuard worker.
- Check that `sysupgrade -l` includes `/etc/routeguard/state.json` and `/etc/routeguard/state.json.pending` when present. After a test firmware upgrade with the package included, protection should recover before RouteGuard resolves fresh addresses.
- With cron disabled, attempts to enable scheduling should report an error. Enable cron, apply again, and check `schedule.running`, `schedule.enabled`, and `schedule.installed`.
- Check that `option enabled 'true'` and `option enabled 'yes'` are accepted. A typo such as `'tru'` should fail while preserving the current rules.

## Size and memory

Compare free space in `/overlay` before and after installing both APKs on the target router. Use the same OpenWrt image for both measurements to account for built-in dependencies.

For RAM tests, apply one profile with a remote list and working DNS, then repeat with several profiles. In a second SSH session, record `VmHWM` and `VmRSS` from `/proc/<pid>/status` for `routeguard apply` and the short-lived `nft` process separately. RouteGuard runs on demand between updates. To estimate kernel memory use, compare the system before applying and after loading the table under the same background workload.

See the [Linux ARM64 benchmarks](docs/RESOURCES.en.md) for the measurement procedure and its limits. Binary size, process RSS, and kernel table memory measure different resources.
