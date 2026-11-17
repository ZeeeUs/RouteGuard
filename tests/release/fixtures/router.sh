#!/bin/sh
set -eu

read_file() {
  if [ "$1" = /etc/apk/arch ]; then
    printf '%s\n' "$TEST_ARCH"
  else
    exec /bin/cat "$@"
  fi
}

read_release_field() {
  case "$1" in
    '@.tag_name') printf '%s\n' "$TEST_TAG" ;;
    '@.assets[@.name="SHA256SUMS"].id') printf '1\n' ;;
    '@.assets[@.name="SHA256SUMS.sig"].id') printf '2\n' ;;
    "@.assets[@.name=\"routeguard-$TEST_ARCH.apk\"].id") printf '3\n' ;;
    '@.assets[@.name="luci-app-routeguard.apk"].id') printf '4\n' ;;
    *) return 1 ;;
  esac
}

read_json_field() {
  case "$2" in
    '@.release.distribution') printf '%s\n' "$TEST_DISTRIBUTION" ;;
    '@.release.version') printf '%s\n' "$TEST_VERSION" ;;
    '@.release.target') printf '%s\n' "$TEST_TARGET" ;;
    release.json) read_release_field "$4" ;;
    *) return 1 ;;
  esac
}

download_fixture() {
  while [ "$1" != -O ]; do
    shift
  done

  case "$3" in
    */releases/latest | */releases/tags/"$TEST_TAG") printf '{}\n' > "$2" ;;
    */assets/[1-4])
      printf '%s\n' "$2" >> "$TEST_LOG"
      cp "$TEST_ASSETS/$2" "$2"
      ;;
    *) return 1 ;;
  esac
}

main() {
  case "$(basename "$0")" in
    id) printf '0\n' ;;
    ubus) printf '{}\n' ;;
    cat) read_file "$@" ;;
    jsonfilter) read_json_field "$@" ;;
    wget) download_fixture "$@" ;;
    apk) printf 'apk %s\n' "$*" >> "$TEST_LOG" ;;
    *) return 1 ;;
  esac
}

main "$@"
