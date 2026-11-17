#!/bin/sh
set -eu

require_test_container() {
  [ -f /.dockerenv ] || {
    echo 'Use make resources (isolated Docker container).' >&2
    exit 1
  }
}

install_test_tools() {
  mkdir -p /etc/config /etc/init.d /etc/crontabs /etc/routeguard /out
  printf '127.0.0.1 127.0.0.1/32\n' > /etc/routeguard/private-list-hosts
  cp /src/build/routeguard /usr/sbin/routeguard
  cp /src/tests/integration/fixtures/uci /usr/local/bin/uci
  cp /src/tests/integration/fixtures/cron /etc/init.d/cron
  chmod +x /usr/sbin/routeguard /usr/local/bin/uci /etc/init.d/cron
}

configure_test_interfaces() {
  ip link set lo up
  index=0
  while [ "$index" -lt 32 ]; do
    ip link add "wan$index" type dummy
    ip link set "wan$index" up
    index=$((index + 1))
  done
}

configure_test_certificate() {
  openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=RouteGuard integration fixture' \
    -addext 'subjectAltName=IP:127.0.0.1' -keyout /tmp/resource-key.pem -out /tmp/resource-cert.pem > /out/tls-setup.log 2>&1
  cp /tmp/resource-cert.pem /usr/local/share/ca-certificates/routeguard-fixture.crt
  update-ca-certificates >> /out/tls-setup.log 2>&1
}

start_fixture_server() {
  python3 /src/tests/integration/resource-server.py &
  server_pid=$!
  trap 'kill "$server_pid" 2>/dev/null || true' EXIT
  attempt=0
  while [ ! -f /tmp/resource-ready ]; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 50 ] || exit 1
    sleep 0.1
  done
}

main() {
  require_test_container
  install_test_tools
  configure_test_interfaces
  configure_test_certificate
  start_fixture_server

  python3 /src/tests/integration/resources.py
}

main "$@"
