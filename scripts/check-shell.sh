#!/bin/sh
set -eu

check_shell_syntax() {
  for script; do
    sh -n "$script"
  done
}

check_shell_syntax \
  install.sh \
  routeguard/files/routeguard.init \
  routeguard/files/routeguard.hotplug \
  luci-app-routeguard/root/usr/libexec/rpcd/luci.routeguard \
  scripts/*.sh \
  tests/integration/*.sh \
  tests/integration/fixtures/uci \
  tests/integration/fixtures/cron \
  tests/openwrt/*.sh \
  tests/release/fixtures/*.sh
