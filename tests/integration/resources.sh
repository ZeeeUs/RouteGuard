#!/bin/sh
set -eu

[ -f /.dockerenv ] || {
  echo 'Use make resources (isolated Docker container).' >&2
  exit 1
}
mkdir -p /etc/config /etc/init.d /etc/crontabs /out
cp /src/build/routeguard /usr/sbin/routeguard
cp /src/tests/integration/fixtures/uci /usr/local/bin/uci
cp /src/tests/integration/fixtures/cron /etc/init.d/cron
chmod +x /usr/sbin/routeguard /usr/local/bin/uci /etc/init.d/cron
ip link set lo up
index=0
while [ "$index" -lt 32 ]; do
  ip link add "wan$index" type dummy
  ip link set "wan$index" up
  index=$((index + 1))
done

openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=RouteGuard integration fixture' \
  -addext 'subjectAltName=IP:127.0.0.1' -keyout /tmp/resource-key.pem -out /tmp/resource-cert.pem > /out/tls-setup.log 2>&1
cp /tmp/resource-cert.pem /usr/local/share/ca-certificates/routeguard-fixture.crt
update-ca-certificates >> /out/tls-setup.log 2>&1
python3 /src/tests/integration/resource-server.py &
server_pid=$!
trap 'kill "$server_pid" 2>/dev/null || true' EXIT
attempt=0
while [ ! -f /tmp/resource-ready ]; do
  attempt=$((attempt + 1))
  [ "$attempt" -lt 50 ] || exit 1
  sleep 0.1
done

python3 /src/tests/integration/resources.py
