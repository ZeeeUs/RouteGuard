#!/bin/sh
set -eu

for script in install.sh routeguard/files/routeguard.init routeguard/files/routeguard.hotplug \
  luci-app-routeguard/root/usr/libexec/rpcd/luci.routeguard scripts/*.sh tests/integration/*.sh tests/openwrt/*.sh \
  tests/integration/fixtures/uci tests/integration/fixtures/cron; do
  sh -n "$script"
done
