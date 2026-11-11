#!/bin/sh
set -eu

repo=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
sdk=${1:?provide a writable SDK directory}
dist=${2:?provide an output directory}
. "$repo/scripts/sdk.env"

mkdir -p "$sdk" "$dist"
sdk=$(CDPATH='' cd -- "$sdk" && pwd)
dist=$(CDPATH='' cd -- "$dist" && pwd)
if [ -f "$sdk/rules.mk" ] && [ ! -f "$sdk/.routeguard-sdk" ]; then
  echo 'Use an empty directory or an SDK created by this script.' >&2
  exit 1
fi
if [ ! -f "$sdk/rules.mk" ]; then
  archive="$sdk/$SDK_ARCHIVE"
  curl --fail --silent --show-error --location --retry 3 "$SDK_URL/$SDK_ARCHIVE" -o "$archive"
  printf '%s  %s\n' "$SDK_SHA256" "$archive" | sha256sum -c -
  tar --zstd -xf "$archive" -C "$sdk" --strip-components=1
  rm "$archive"
  touch "$sdk/.routeguard-sdk"
fi

cd "$sdk"
./scripts/feeds update base packages luci
# This is a dedicated build directory. Avoid SDK defaults that select every
# package, kernel module and device bootloader from every feed.
rm -rf package/feeds tmp
./scripts/feeds install firewall4 ca-bundle luci-base
mkdir -p package/routeguard-local
for package in routeguard luci-app-routeguard; do
  rsync -a --delete "$repo/$package/" "package/routeguard-local/$package/"
done
cp "$repo/scripts/sdk.config" .config
make defconfig
for package in routeguard luci-app-routeguard; do
  grep -qx "CONFIG_PACKAGE_$package=m" .config || {
    echo "SDK did not select $package; check its package recipe." >&2
    exit 1
  }
done
# Both packages contain only a static Go executable or JS/shell assets. Runtime
# dependencies come from the matching OpenWrt repositories; do not rebuild the
# firmware's kernel modules and system packages. Build LuCI's host minifier
# explicitly before using the SDK's NO_DEPS package target.
make -j1 package/feeds/luci/luci-base/host/compile NO_DEPS=1 V=s
make -j1 package/routeguard-local/routeguard/compile NO_DEPS=1 V=s
make -j1 package/routeguard-local/luci-app-routeguard/compile NO_DEPS=1 CONFIG_LUCI_CSSTIDY= V=s

for package in routeguard luci-app-routeguard; do
  version=$(sed -n 's/^PKG_VERSION:=//p' "$repo/$package/Makefile")
  release=$(sed -n 's/^PKG_RELEASE:=//p' "$repo/$package/Makefile")
  matches=$(find bin/packages -type f -name "$package-$version-r$release.apk")
  [ "$(printf '%s\n' "$matches" | wc -l)" -eq 1 ] && [ -f "$matches" ]
  asset="$package.apk"
  if [ "$package" = routeguard ]; then
    case "$matches" in */aarch64_cortex-a53/*) ;; *) exit 1 ;; esac
    asset=routeguard-aarch64_cortex-a53.apk
  fi
  cp "$matches" "$dist/$asset"
done

cp "$repo/install.sh" "$dist/"
cd "$dist"
sha256sum routeguard-aarch64_cortex-a53.apk luci-app-routeguard.apk > SHA256SUMS
python3 "$repo/scripts/check-package-size.py" "$dist"
