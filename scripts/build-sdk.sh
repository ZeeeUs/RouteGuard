#!/bin/sh
set -eu

repo=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
sdk=${1:?provide a writable SDK directory}
dist=${2:?provide an output directory}
tag=${3-}
architecture=${4:-aarch64_cortex-a53}
parameters=$(python3 "$repo/scripts/release.py" sdk "$architecture")
read -r SDK_VERSION SDK_TARGET SDK_SUBTARGET SDK_ARCHIVE SDK_SHA256 BUILD_LUCI << EOF
$parameters
EOF
SDK_URL="https://downloads.openwrt.org/releases/$SDK_VERSION/targets/$SDK_TARGET/$SDK_SUBTARGET"

version=0.0.0
if [ -n "$tag" ]; then
  if ! printf '%s\n' "$tag" | awk 'NR != 1 || $0 !~ /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/ { exit 1 }'; then
    printf 'Invalid release tag "%s". Expected vMAJOR.MINOR.PATCH.\n' "$tag" >&2
    exit 1
  fi
  version=${tag#v}
fi
export ROUTEGUARD_VERSION="$version"
printf 'Building RouteGuard packages with version %s\n' "$version"

mkdir -p "$sdk" "$dist"
sdk=$(CDPATH='' cd -- "$sdk" && pwd)
dist=$(CDPATH='' cd -- "$dist" && pwd)
if [ -f "$sdk/rules.mk" ] && [ ! -f "$sdk/.routeguard-sdk" ]; then
  echo 'Use an empty directory or an SDK created by this script.' >&2
  exit 1
fi
identity="$SDK_VERSION:$architecture:$SDK_SHA256"
if [ -f "$sdk/.routeguard-sdk" ] && [ "$(cat "$sdk/.routeguard-sdk")" != "$identity" ]; then
  echo 'SDK directory belongs to another build. Use a fresh directory.' >&2
  exit 1
fi
if [ ! -f "$sdk/rules.mk" ]; then
  archive="$sdk/$SDK_ARCHIVE"
  curl --fail --silent --show-error --location --retry 3 "$SDK_URL/$SDK_ARCHIVE" -o "$archive"
  printf '%s  %s\n' "$SDK_SHA256" "$archive" | sha256sum -c -
  tar --zstd -xf "$archive" -C "$sdk" --strip-components=1
  rm "$archive"
  printf '%s\n' "$identity" > "$sdk/.routeguard-sdk"
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
{
  printf 'CONFIG_TARGET_%s=y\nCONFIG_TARGET_%s_%s=y\n' "$SDK_TARGET" "$SDK_TARGET" "$SDK_SUBTARGET"
  cat "$repo/scripts/sdk.config"
} > .config
packages=routeguard
if [ "$BUILD_LUCI" = 1 ]; then
  printf 'CONFIG_PACKAGE_luci-app-routeguard=m\n' >> .config
  packages="$packages luci-app-routeguard"
fi
make defconfig
grep -qx "CONFIG_TARGET_ARCH_PACKAGES=\"$architecture\"" .config || {
  echo "SDK package architecture does not match $architecture" >&2
  exit 1
}
for package in $packages; do
  grep -qx "CONFIG_PACKAGE_$package=m" .config || {
    echo "SDK did not select $package; check its package recipe." >&2
    exit 1
  }
done
# Both packages contain only a static Go executable or JS/shell assets. Runtime
# dependencies come from the matching OpenWrt repositories; do not rebuild the
# firmware's kernel modules and system packages. Build LuCI's host minifier
# explicitly before using the SDK's NO_DEPS package target.
if [ "$BUILD_LUCI" = 1 ]; then
  make -j1 package/feeds/luci/luci-base/host/compile NO_DEPS=1 V=s
fi
make -j1 package/routeguard-local/routeguard/compile NO_DEPS=1 V=s
if [ "$BUILD_LUCI" = 1 ]; then
  make -j1 package/routeguard-local/luci-app-routeguard/compile NO_DEPS=1 CONFIG_LUCI_CSSTIDY= V=s
fi

for package in $packages; do
  release=$(sed -n 's/^PKG_RELEASE:=//p' "$repo/$package/Makefile")
  filename="$package-$version-r$release.apk"
  matches=$(find bin/packages -type f -name "$filename")
  if [ "$(printf '%s\n' "$matches" | wc -l)" -ne 1 ] || [ ! -f "$matches" ]; then
    printf 'Expected exactly one built APK named %s.\n' "$filename" >&2
    exit 1
  fi
  asset="$package.apk"
  package_arch=noarch
  if [ "$package" = routeguard ]; then
    case "$matches" in */"$architecture"/*) ;; *) exit 1 ;; esac
    asset="routeguard-$architecture.apk"
    package_arch="$architecture"
  fi
  python3 "$repo/scripts/check-apk.py" staging_dir/host/bin/apk "$matches" "$package_arch"
  cp "$matches" "$dist/$asset"
done

cp "$repo/install.sh" "$dist/"
cd "$dist"
for asset in *.apk; do sha256sum "$asset"; done > SHA256SUMS
python3 "$repo/scripts/check-package-size.py" "$dist"
