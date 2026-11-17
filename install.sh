#!/bin/sh
set -eu

release_api=https://api.github.com/repos/ZeeeUs/RouteGuard/releases
# Filled from the signing key when the release installer is generated.
embedded_public_key=''
# Trusted release key for the installer downloaded directly from main.
source_public_key='RWSUfCh6mMGdvzgroob95sGjtvLjjvIALJoVipTllJojlSVq8rx5T+ft'

fail() {
  printf 'RouteGuard: %s\n' "$*" >&2
  exit 1
}

valid_tag() {
  printf '%s\n' "$1" | awk 'NR != 1 || $0 !~ /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/ { exit 1 }'
}

check_requirements() {
  [ "$(id -u)" = 0 ] || fail "run the installer as root on the router"
  for tool in apk ubus jsonfilter wget sha256sum awk cat usign; do
    command -v "$tool" > /dev/null 2>&1 || fail "missing command: $tool"
  done
}

detect_router() {
  board=$(ubus call system board) || fail "could not identify the firmware"
  distribution=$(printf '%s\n' "$board" | jsonfilter -e '@.release.distribution')
  version=$(printf '%s\n' "$board" | jsonfilter -e '@.release.version')
  target=$(printf '%s\n' "$board" | jsonfilter -e '@.release.target')
  # apk --print-arch reports the compiler architecture. Use the installed package ABI.
  arch=$(cat /etc/apk/arch) || fail "could not identify the OpenWrt package architecture"

  case "$distribution:$version" in
    OpenWrt:25.12.*) ;;
    *) fail "unsupported firmware: $distribution $version ($target, $arch)" ;;
  esac

  case "$arch" in
    aarch64_cortex-a53 | aarch64_generic | aarch64_cortex-a72 | aarch64_cortex-a76 | x86_64) ;;
    *) fail "no release build for $distribution $version ($target, $arch)" ;;
  esac
}

prepare_workspace() {
  umask 077
  key=${ROUTEGUARD_RELEASE_KEY:-/etc/routeguard/release.pub}
  case "$key" in
    /*) ;;
    *) key="$PWD/$key" ;;
  esac

  workspace=$(mktemp -d /tmp/routeguard.XXXXXX) || fail "could not create a temporary directory"
  trap 'rm -rf "$workspace"' EXIT
  trap 'exit 1' HUP INT TERM
  cd "$workspace"
}

select_verification_key() {
  if [ -n "${ROUTEGUARD_RELEASE_KEY:-}" ] || [ -e "$key" ] || [ -L "$key" ]; then
    [ -f "$key" ] && [ -r "$key" ] || fail "cannot read the configured RouteGuard release public key at $key"
    return
  fi

  key=release.pub
  printf '%s\n' 'untrusted comment: RouteGuard release key' "${embedded_public_key:-$source_public_key}" > "$key"
}

fetch_release() {
  release_path=latest
  [ -z "$requested_tag" ] || release_path="tags/$requested_tag"

  wget -q -T 30 -O release.json "$release_api/$release_path" || fail "could not read the GitHub release"
  tag=$(jsonfilter -i release.json -e '@.tag_name') || fail "invalid GitHub release response"
  valid_tag "$tag" || fail "GitHub release has an invalid tag"
  [ -z "$requested_tag" ] || [ "$requested_tag" = "$tag" ] || fail "GitHub release tag does not match $requested_tag"
  printf 'Downloading prebuilt RouteGuard %s packages for %s.\n' "$tag" "$arch"
}

download_asset() {
  # Every asset is resolved from the same release, using API downloads for OpenWrt wget.
  asset_id=$(jsonfilter -i release.json -e "@.assets[@.name=\"$1\"].id") \
    || fail "could not find $1 in release $tag"
  case "$asset_id" in
    '' | *[!0-9]*) fail "could not find a unique $1 in release $tag" ;;
  esac

  wget -q -T 30 --header='Accept: application/octet-stream' -O "$1" "$release_api/assets/$asset_id" \
    || fail "could not download $1 from release $tag"
}

verify_manifest() {
  download_asset SHA256SUMS
  download_asset SHA256SUMS.sig
  [ "$(wc -c < SHA256SUMS)" -le 4096 ] && [ "$(wc -c < SHA256SUMS.sig)" -le 1024 ] \
    || fail "invalid release signature or manifest size"

  usign -q -V -p "$key" -m SHA256SUMS -x SHA256SUMS.sig \
    || fail "release signature did not match the trusted RouteGuard key"
  IFS= read -r signed_tag < SHA256SUMS || fail "manifest has no signed release tag"
  [ "$signed_tag" = "# RouteGuard $tag" ] || fail "signed manifest does not match release $tag"
}

download_verified_package() {
  # Verify only the expected asset. A manifest cannot add paths to read.
  awk -v file="$1" '
    NF == 2 && ($2 == file || $2 == "*" file) &&
    length($1) == 64 && $1 !~ /[^a-fA-F0-9]/ { print $1 "  " file; count++ }
    END { if (count != 1) exit 1 }
  ' SHA256SUMS > "$1.sha256" || fail "invalid checksum entry for $1"

  download_asset "$1"
  sha256sum -c "$1.sha256" || fail "checksum mismatch for $1"
}

install_packages() {
  apk update || fail "could not refresh the OpenWrt package indexes"
  apk add --allow-untrusted "./$1" "./$2" || fail "APK installation failed"

  if [ -x /etc/init.d/rpcd ]; then
    /etc/init.d/rpcd restart || fail "packages installed; restart rpcd manually"
  fi
  printf '%s\n' 'RouteGuard installed. Open Services → RouteGuard in LuCI.'
}

main() {
  [ "$#" -le 1 ] || fail "usage: sh install.sh [vMAJOR.MINOR.PATCH]"
  requested_tag=${1-}
  [ -z "$requested_tag" ] || valid_tag "$requested_tag" || fail "invalid release tag: $requested_tag"

  check_requirements
  detect_router
  engine=routeguard-$arch.apk
  ui=luci-app-routeguard.apk

  prepare_workspace
  select_verification_key
  fetch_release
  verify_manifest
  download_verified_package "$engine"
  download_verified_package "$ui"
  install_packages "$engine" "$ui"
}

main "$@"
