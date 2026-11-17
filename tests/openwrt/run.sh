#!/bin/sh
set -eu

require_openwrt_container() {
  # Use only a disposable OpenWrt rootfs container, never an actual router.
  [ -f /.dockerenv ] || exit 1
  . /etc/openwrt_release
  case "$DISTRIB_ID:$DISTRIB_RELEASE" in
    OpenWrt:25.12.*) ;;
    *) exit 1 ;;
  esac
}

wait_for() {
  attempt=0
  until "$@"; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 50 ] || exit 1
    sleep 1
  done
}

stop_services() {
  set -- "$bus_pid"
  if [ -n "$rpc_pid" ]; then
    set -- "$rpc_pid" "$@"
  fi
  kill "$@" 2> /dev/null || true
}

start_bus() {
  mkdir -p /var/run/ubus /var/lock /var/log /tmp/.uci /out
  ubusd &
  bus_pid=$!
  rpc_pid=''
  trap stop_services EXIT
  wait_for ubus list > /dev/null 2>&1
}

install_packages() {
  architecture=$(cat /etc/apk/arch)
  apk add --no-network --allow-untrusted "/packages/routeguard-$architecture.apk" /packages/luci-app-routeguard.apk
  test -x /usr/libexec/rpcd/luci.routeguard
  test -f /usr/share/routeguard/staged-config.uc
  test -x /etc/hotplug.d/iface/95-routeguard
  test -f /lib/upgrade/keep.d/routeguard
  test -e /etc/rc.d/S18routeguard
  test ! -f /etc/routeguard/state.json
  test "$(uci show routeguard)" = 'routeguard.main=settings'
}

configure_test_accounts() {
  # Isolated fixture accounts with the test password "fixture-password".
  cat >> /etc/config/rpcd << 'EOF'

config login
	option username 'routeguard-reader'
	option password '$1$rgtest$AONMA4rbIW.WObujqoL3B1'
	list read 'luci-base'
	list read 'luci-app-routeguard'

config login
	option username 'routeguard-editor'
	option password '$1$rgtest$AONMA4rbIW.WObujqoL3B1'
	list read 'luci-base'
	list read 'luci-app-routeguard'
	list write 'luci-app-routeguard'
EOF
}

rpc_is_ready() {
  ubus list luci.routeguard 2> /dev/null | grep -qx luci.routeguard
}

start_rpc() {
  rpcd &
  rpc_pid=$!
  wait_for rpc_is_ready
}

check_blank_status() {
  ubus call luci.routeguard status > /out/openwrt-blank.json
  test "$(jsonfilter -i /out/openwrt-blank.json -e '@.loaded')" = false
  ubus call luci.routeguard devices | jsonfilter -e '@.devices[*]' | grep -qx lo
}

session() {
  ubus call session login "{\"username\":\"$1\",\"password\":\"fixture-password\"}" \
    | jsonfilter -e '@.ubus_rpc_session'
}

session_access() {
  ubus call session access "{\"ubus_rpc_session\":\"$1\",\"scope\":\"$2\",\"object\":\"$3\",\"function\":\"$4\"}" \
    | jsonfilter -e '@.access'
}

check_read_only_access() {
  reader=$(session routeguard-reader)
  test -n "$reader"
  test "$(session_access "$reader" uci routeguard read)" = true
  test "$(session_access "$reader" uci wireless read)" = false
  test "$(session_access "$reader" uci network read)" = false
  test "$(session_access "$reader" ubus luci.routeguard apply)" = false
  test "$(session_access "$reader" ubus luci.routeguard devices)" = true
}

apply_manual_configuration() {
  cp /src/tests/integration/fixtures/manual.uci /etc/config/routeguard
  sed -i "s/'wan'/'lo'/; s/'wan2'/'lo'/" /etc/config/routeguard
  uci set firewall.@defaults[0].flow_offloading=0
  uci set firewall.@defaults[0].flow_offloading_hw=0
  uci commit firewall
  routeguard apply
  ubus call luci.routeguard status > /out/openwrt-applied.json
  test "$(jsonfilter -i /out/openwrt-applied.json -e '@.loaded')" = true
}

queue_result() {
  id=$(printf '%s' "$1" | jsonfilter -e '@.id')
  test -n "$id"
  attempt=0
  while :; do
    result=$(ubus call luci.routeguard job "{\"id\":\"$id\"}")
    state=$(printf '%s' "$result" | jsonfilter -e '@.state')
    if [ "$state" != running ]; then
      printf '%s\n' "$result"
      return
    fi
    attempt=$((attempt + 1))
    [ "$attempt" -lt 50 ] || exit 1
    sleep 1
  done
}

check_job() {
  expected_state=$1
  output_file=$2
  shift 2

  queue_result "$(ubus call luci.routeguard "$@")" > "$output_file"
  test "$(jsonfilter -i "$output_file" -e '@.state')" = "$expected_state"
}

check_apply_revision() {
  editor=$(session routeguard-editor)
  test -n "$editor"
  ubus call uci add "{\"ubus_rpc_session\":\"$editor\",\"config\":\"routeguard\",\"type\":\"service\",\"values\":{\"label\":\"Anonymous fixture\"}}" > /dev/null
  revision=$(ubus call luci.routeguard revision "{\"ubus_rpc_session\":\"$editor\"}" | jsonfilter -e '@.revision')
  test "${#revision}" -eq 64
  ubus call uci commit "{\"ubus_rpc_session\":\"$editor\",\"config\":\"routeguard\"}"
  check_job ok /out/openwrt-apply.json apply "{\"revision\":\"$revision\"}"
  check_job ok /out/openwrt-diagnose.json diagnose

  uci set 'routeguard.@service[-1].label=Changed fixture'
  uci commit routeguard
  check_job error /out/openwrt-conflict.json apply "{\"revision\":\"$revision\"}"
  check_job error /out/openwrt-diagnose-conflict.json diagnose
  jsonfilter -i /out/openwrt-diagnose-conflict.json -e '@.message' | grep -q 'settings differ from active rules'
  test "$(ubus call luci.routeguard status | jsonfilter -e '@.loaded')" = true
}

check_refresh_and_restore() {
  check_job ok /out/openwrt-job.json refresh

  nft flush chain inet routeguard forward
  test "$(routeguard status | jsonfilter -e '@.loaded')" = false
  check_job ok /out/openwrt-restore.json restore
  test "$(routeguard status | jsonfilter -e '@.loaded')" = true
}

check_sysupgrade_keep_list() {
  printf '{"version":1,"target":%s}\n' "$(cat /etc/routeguard/state.json)" > /etc/routeguard/state.json.pending
  sysupgrade -l > /out/openwrt-backup-files.txt
  grep -qx /etc/routeguard/state.json /out/openwrt-backup-files.txt
  grep -qx /etc/routeguard/state.json.pending /out/openwrt-backup-files.txt
  rm /etc/routeguard/state.json.pending
}

remove_packages() {
  apk del --no-network luci-app-routeguard routeguard
  test ! -e /usr/sbin/routeguard
  if nft list table inet routeguard > /dev/null 2>&1; then exit 1; fi
}

main() {
  require_openwrt_container
  start_bus
  install_packages
  configure_test_accounts
  start_rpc

  check_blank_status
  check_read_only_access
  apply_manual_configuration
  check_apply_revision
  check_refresh_and_restore
  check_sysupgrade_keep_list
  remove_packages

  printf '%s\n' 'PASS: OpenWrt APK install/remove, blank config, init enable, native nft, UCI, RPC operations and sysupgrade keep list.'
}

main "$@"
