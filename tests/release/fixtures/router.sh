#!/bin/sh
set -eu

case "$(basename "$0")" in
  id) printf '0\n' ;;
  ubus) printf '{}\n' ;;
  cat) printf '%s\n' "$TEST_ARCH" ;;
  jsonfilter)
    case "$2" in
      '@.release.distribution') printf '%s\n' "$TEST_DISTRIBUTION" ;;
      '@.release.version') printf '%s\n' "$TEST_VERSION" ;;
      '@.release.target') printf '%s\n' "$TEST_TARGET" ;;
      *) exit 1 ;;
    esac
    ;;
  *) exit 1 ;;
esac
