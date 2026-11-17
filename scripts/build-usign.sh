#!/bin/sh
set -eu

# Use the same usign source as the OpenWrt 25.12.5 SDK, with standalone base64.
revision=c4c72b1b07945ee192361dc751291a7c98d6adcd

fetch_sources() {
  git init -q "$1"
  git -C "$1" fetch -q --depth 1 https://github.com/openwrt/usign.git "$revision"
  git -C "$1" checkout -q --detach "$revision"
}

main() {
  work=${1:?provide an empty work directory}
  mkdir -p "$work"
  fetch_sources "$work/src"
  cmake -S "$work/src" -B "$work/bin" -DUSE_LIBUBOX=OFF
  cmake --build "$work/bin"
}

main "$@"
