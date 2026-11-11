#!/bin/sh
set -eu

fail() {
  printf 'RouteGuard: %s\n' "$*" >&2
  exit 1
}

[ "$(id -u)" = 0 ] || fail "run the installer as root on the router"
for tool in apk ubus jsonfilter wget sha256sum awk cat; do
  command -v "$tool" > /dev/null 2>&1 || fail "missing command: $tool"
done

board=$(ubus call system board) || fail "could not identify the firmware"
distribution=$(printf '%s\n' "$board" | jsonfilter -e '@.release.distribution')
version=$(printf '%s\n' "$board" | jsonfilter -e '@.release.version')
target=$(printf '%s\n' "$board" | jsonfilter -e '@.release.target')
# --print-arch reports apk's compiler architecture (for example aarch64), not
# OpenWrt's package ABI. The installed package architecture is recorded here.
arch=$(cat /etc/apk/arch) || fail "could not identify the OpenWrt package architecture"

case "$distribution:$version:$target:$arch" in
  OpenWrt:25.12.*:mediatek/filogic:aarch64_cortex-a53) ;;
  *) fail "no release build for $distribution $version ($target, $arch); build from source" ;;
esac

umask 077
tmp=$(mktemp -d /tmp/routeguard.XXXXXX) || fail "could not create a temporary directory"
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM
cd "$tmp"

release=https://github.com/ZeeeUs/RouteGuard/releases/latest/download
engine=routeguard-$arch.apk
ui=luci-app-routeguard.apk

download() {
  wget -T 30 -O "$1" "$release/$1" \
    || fail "could not download $1 from the latest published release"
}

download SHA256SUMS
for package in "$engine" "$ui"; do
  # Verify only the two expected assets. A manifest cannot add paths to read.
  awk -v file="$package" '
        NF == 2 && ($2 == file || $2 == "*" file) &&
        length($1) == 64 && $1 !~ /[^a-fA-F0-9]/ { print $1 "  " file; count++ }
        END { if (count != 1) exit 1 }
    ' SHA256SUMS > "$package.sha256" || fail "invalid checksum entry for $package"
  download "$package"
  sha256sum -c "$package.sha256" || fail "checksum mismatch for $package"
done

apk update || fail "could not refresh the OpenWrt package indexes"
apk add --allow-untrusted "./$engine" "./$ui" || fail "APK installation failed"

if [ -x /etc/init.d/rpcd ]; then
  /etc/init.d/rpcd restart || fail "packages installed; restart rpcd manually"
fi
printf '%s\n' 'RouteGuard installed. Open Services → RouteGuard in LuCI.'
