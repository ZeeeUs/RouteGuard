#!/bin/sh
set -eu

fail() {
  printf '%s\n' "$*" >&2
  exit 1
}

load_target() {
  parameters=$(python3 "$repo/scripts/release.py" sdk "$architecture")
  read -r SDK_VERSION SDK_TARGET SDK_SUBTARGET SDK_ARCHIVE SDK_SHA256 BUILD_LUCI << EOF_TARGET
$parameters
EOF_TARGET
  sdk_url="https://downloads.openwrt.org/releases/$SDK_VERSION/targets/$SDK_TARGET/$SDK_SUBTARGET"
}

select_version() {
  version=0.0.0
  if [ -n "$tag" ]; then
    printf '%s\n' "$tag" | awk 'NR != 1 || $0 !~ /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/ { exit 1 }' \
      || fail "Invalid release tag \"$tag\". Expected vMAJOR.MINOR.PATCH."
    version=${tag#v}
  fi

  export ROUTEGUARD_VERSION="$version"
  printf 'Building RouteGuard packages with version %s\n' "$version"
}

prepare_sdk() {
  mkdir -p "$sdk" "$dist"
  sdk=$(CDPATH='' cd -- "$sdk" && pwd)
  dist=$(CDPATH='' cd -- "$dist" && pwd)

  identity="$SDK_VERSION:$architecture:$SDK_SHA256"
  if [ -f "$sdk/rules.mk" ] && [ ! -f "$sdk/.routeguard-sdk" ]; then
    fail 'Use an empty directory or an SDK created by this script.'
  fi
  if [ -f "$sdk/.routeguard-sdk" ] && [ "$(cat "$sdk/.routeguard-sdk")" != "$identity" ]; then
    fail 'SDK directory belongs to another build. Use a fresh directory.'
  fi
  if [ -f "$sdk/rules.mk" ]; then
    return
  fi

  archive="$sdk/$SDK_ARCHIVE"
  curl --fail --silent --show-error --location --retry 3 "$sdk_url/$SDK_ARCHIVE" -o "$archive"
  printf '%s  %s\n' "$SDK_SHA256" "$archive" | sha256sum -c -
  tar --zstd -xf "$archive" -C "$sdk" --strip-components=1
  rm "$archive"
  printf '%s\n' "$identity" > "$sdk/.routeguard-sdk"
}

prepare_feeds() {
  ./scripts/feeds update base packages luci
  # Use a dedicated build directory and select only the required packages.
  rm -rf package/feeds tmp
  ./scripts/feeds install firewall4 ca-bundle luci-base

  mkdir -p package/routeguard-local
  for recipe in routeguard luci-app-routeguard; do
    rsync -a --delete "$repo/$recipe/" "package/routeguard-local/$recipe/"
  done
}

configure_sdk() {
  {
    printf 'CONFIG_TARGET_%s=y\nCONFIG_TARGET_%s_%s=y\n' "$SDK_TARGET" "$SDK_TARGET" "$SDK_SUBTARGET"
    cat "$repo/scripts/sdk.config"
    if [ "$BUILD_LUCI" = 1 ]; then
      printf 'CONFIG_PACKAGE_luci-app-routeguard=m\n'
    fi
  } > .config

  make defconfig
  grep -qx "CONFIG_TARGET_ARCH_PACKAGES=\"$architecture\"" .config \
    || fail "SDK package architecture does not match $architecture"
  for package; do
    grep -qx "CONFIG_PACKAGE_$package=m" .config \
      || fail "SDK did not select $package; check its package recipe."
  done
}

compile_packages() {
  # Build LuCI's host minifier explicitly before using the SDK's NO_DEPS target.
  if [ "$BUILD_LUCI" = 1 ]; then
    make -j1 package/feeds/luci/luci-base/host/compile NO_DEPS=1 V=s
  fi

  make -j1 package/routeguard-local/routeguard/compile NO_DEPS=1 V=s

  if [ "$BUILD_LUCI" = 1 ]; then
    make -j1 package/routeguard-local/luci-app-routeguard/compile NO_DEPS=1 CONFIG_LUCI_CSSTIDY= V=s
  fi
}

collect_package() {
  release=$(sed -n 's/^PKG_RELEASE:=//p' "$repo/$1/Makefile")
  filename="$1-$version-r$release.apk"
  matches=$(find bin/packages -type f -name "$filename")
  if [ "$(printf '%s\n' "$matches" | wc -l)" -ne 1 ] || [ ! -f "$matches" ]; then
    fail "Expected exactly one built APK named $filename."
  fi

  asset="$1.apk"
  package_arch=noarch
  if [ "$1" = routeguard ]; then
    case "$matches" in
      */"$architecture"/*) ;;
      *) exit 1 ;;
    esac
    asset="routeguard-$architecture.apk"
    package_arch="$architecture"
  fi

  python3 "$repo/scripts/check-apk.py" staging_dir/host/bin/apk "$matches" "$package_arch"
  cp "$matches" "$dist/$asset"
}

write_distribution() {
  for package; do
    collect_package "$package"
  done

  cp "$repo/install.sh" "$dist/"
  cd "$dist"
  for asset in *.apk; do
    sha256sum "$asset"
  done > SHA256SUMS
  python3 "$repo/scripts/check-package-size.py" "$dist"
}

main() {
  repo=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
  sdk=${1:?provide a writable SDK directory}
  dist=${2:?provide an output directory}
  tag=${3-}
  architecture=${4:-aarch64_cortex-a53}

  load_target
  select_version
  prepare_sdk
  cd "$sdk"
  prepare_feeds

  set -- routeguard
  [ "$BUILD_LUCI" != 1 ] || set -- "$@" luci-app-routeguard
  configure_sdk "$@"
  compile_packages
  write_distribution "$@"
}

main "$@"
