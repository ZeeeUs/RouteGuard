#!/bin/sh
set -eu

# Use only a disposable OpenWrt rootfs container, never an actual router.
[ -f /.dockerenv ] || exit 1
. /etc/openwrt_release
[ "$DISTRIB_RELEASE:$DISTRIB_TARGET" = '25.12.5:mediatek/filogic' ] || exit 1
mkdir -p /var/run/ubus /var/lock /var/log /tmp/.uci /out

ubusd &
bus_pid=$!
rpc_pid=''
trap 'kill $rpc_pid "$bus_pid" 2>/dev/null || true' EXIT
attempt=0
until ubus list > /dev/null 2>&1; do
  attempt=$((attempt + 1))
  [ "$attempt" -lt 50 ] || exit 1
  sleep 1
done

apk add --no-network --allow-untrusted /packages/routeguard-aarch64_cortex-a53.apk /packages/luci-app-routeguard.apk
test -x /usr/libexec/rpcd/luci.routeguard
test -x /etc/hotplug.d/iface/95-routeguard
test -f /lib/upgrade/keep.d/routeguard
test -e /etc/rc.d/S18routeguard
test ! -f /etc/routeguard/state.json
test "$(uci show routeguard)" = 'routeguard.main=settings'

rpcd &
rpc_pid=$!
attempt=0
until ubus list luci.routeguard 2> /dev/null | grep -qx luci.routeguard; do
  attempt=$((attempt + 1))
  [ "$attempt" -lt 50 ] || exit 1
  sleep 1
done
ubus call luci.routeguard status > /out/openwrt-blank.json
test "$(jsonfilter -i /out/openwrt-blank.json -e '@.loaded')" = false

cp /src/tests/integration/fixtures/manual.uci /etc/config/routeguard
sed -i "s/'wan'/'lo'/; s/'wan2'/'lo'/" /etc/config/routeguard
uci set firewall.@defaults[0].flow_offloading=0
uci set firewall.@defaults[0].flow_offloading_hw=0
uci commit firewall
routeguard apply
ubus call luci.routeguard status > /out/openwrt-applied.json
test "$(jsonfilter -i /out/openwrt-applied.json -e '@.loaded')" = true

reply=$(ubus call luci.routeguard refresh)
id=$(printf '%s' "$reply" | jsonfilter -e '@.id')
test -n "$id"
attempt=0
while :; do
  result=$(ubus call luci.routeguard job "{\"id\":\"$id\"}")
  state=$(printf '%s' "$result" | jsonfilter -e '@.state')
  [ "$state" = ok ] && break
  [ "$state" = running ] || exit 1
  attempt=$((attempt + 1))
  [ "$attempt" -lt 50 ] || exit 1
  sleep 1
done
printf '%s\n' "$result" > /out/openwrt-job.json

nft flush chain inet routeguard forward
test "$(routeguard status | jsonfilter -e '@.loaded')" = false
routeguard refresh
test "$(routeguard status | jsonfilter -e '@.loaded')" = true
printf '{"version":1,"target":%s}\n' "$(cat /etc/routeguard/state.json)" > /etc/routeguard/state.json.pending
sysupgrade -l > /out/openwrt-backup-files.txt
grep -qx /etc/routeguard/state.json /out/openwrt-backup-files.txt
grep -qx /etc/routeguard/state.json.pending /out/openwrt-backup-files.txt
rm /etc/routeguard/state.json.pending

apk del --no-network luci-app-routeguard routeguard
test ! -e /usr/sbin/routeguard
if nft list table inet routeguard > /dev/null 2>&1; then exit 1; fi
printf '%s\n' 'PASS: OpenWrt APK install/remove, blank config, init enable, native nft, UCI, RPC operations and sysupgrade keep list.'
