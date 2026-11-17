#!/bin/sh
set -eu

require_test_container() {
  # Run only in the disposable, network-isolated test container.
  [ -f /.dockerenv ] || {
    echo 'Use make integration (isolated Docker container).' >&2
    exit 1
  }
}

install_test_tools() {
  mkdir -p /etc/config /etc/init.d /etc/crontabs /out
  cp /src/build/routeguard /usr/sbin/routeguard
  cp /src/tests/integration/fixtures/uci /usr/local/bin/uci
  cp /src/tests/integration/fixtures/cron /etc/init.d/cron
  chmod +x /usr/sbin/routeguard /usr/local/bin/uci /etc/init.d/cron
}

configure_test_link() {
  namespace=$1
  device=$2
  subnet=$3

  ip netns add "$namespace"
  ip link add "$device" type veth peer name peer
  ip link set peer netns "$namespace"
  ip netns exec "$namespace" sysctl -qw net.ipv6.conf.peer.accept_dad=0
  ip addr add "10.$subnet.0.1/24" dev "$device"
  ip -6 addr add "fd00:$subnet::1/64" dev "$device" nodad
  ip link set "$device" up
  ip -n "$namespace" link set lo up
  ip -n "$namespace" link set peer up
  ip -n "$namespace" addr add "10.$subnet.0.2/24" dev peer
  ip -n "$namespace" -6 addr add "fd00:$subnet::2/64" dev peer nodad
  ip -n "$namespace" route add default via "10.$subnet.0.1"
  ip -n "$namespace" -6 route add default via "fd00:$subnet::1"
}

configure_network() {
  ip link set lo up
  sysctl -qw net.ipv4.ip_forward=1
  sysctl -qw net.ipv6.conf.all.forwarding=1
  sysctl -qw net.ipv6.conf.default.accept_dad=0
  configure_test_link client lan 1
  configure_test_link outside wan 2
  configure_test_link outside2 wan2 3

  for host in 3 4; do
    ip -n outside addr add "10.2.0.$host/24" dev peer
    ip -n outside -6 addr add "fd00:2::$host/64" dev peer nodad
  done
}

allowed() {
  "$@" > /dev/null 2>&1 || {
    echo "Unexpectedly blocked: $*" >&2
    "$@"
    exit 1
  }
}
blocked() {
  if "$@" > /dev/null 2>&1; then
    echo "Traffic escaped: $*" >&2
    exit 1
  fi
}
verified() {
  routeguard status | python3 -c 'import json,sys; s=json.load(sys.stdin); assert s.get("loaded"), s'
}
drifted() {
  routeguard status | python3 -c 'import json,sys; s=json.load(sys.stdin); assert s.get("table_present") and not s.get("loaded"), s'
}

check_blank_configuration() {
  nft add table inet fw4
  cp /src/tests/integration/fixtures/empty.uci /etc/config/routeguard
  routeguard restore
  test ! -f /etc/routeguard/state.json
  test ! -f /etc/crontabs/root
}

check_wan_filtering() {
  allowed ping -c 1 -W 1 10.2.0.2
  allowed ip netns exec client ping -6 -c 1 -W 1 fd00:2::2
  cp /src/tests/integration/fixtures/manual.uci /etc/config/routeguard
  routeguard apply
  verified
  nft -j -n list table inet routeguard > /out/actual-table.json
  for address in 10.2.0.2 10.3.0.2 fd00:2::2 fd00:3::2; do
    blocked ping -c 1 -W 1 "$address"
    blocked ip netns exec client ping -c 1 -W 1 "$address"
  done
  for address in 10.2.0.3 10.2.0.4 fd00:2::3 fd00:2::4; do
    allowed ping -c 1 -W 1 "$address"
    allowed ip netns exec client ping -c 1 -W 1 "$address"
  done
}

check_refresh_counters() {
  routeguard status > /out/before-refresh.json
  routeguard refresh
  routeguard status > /out/after-refresh.json
  python3 - << 'PY'
import json
before = json.load(open('/out/before-refresh.json'))
after = json.load(open('/out/after-refresh.json'))
assert before['counters'] == after['counters'], 'unchanged refresh reset counters'
for name in ('forward4', 'forward6', 'output4', 'output6'):
    assert before['counters'][name]['packets'] > 0, name
PY
}

check_repair_after() {
  "$@"
  drifted
  routeguard refresh
  verified
}

check_firewall_repair() {
  check_repair_after nft flush chain inet routeguard forward
  check_repair_after nft flush set inet routeguard blocked4
  check_repair_after nft add chain inet routeguard unexpected

  nft delete table inet fw4
  nft add table inet fw4
  verified
}

check_invalid_configuration() {
  sed -i "s/enabled 'true'/enabled 'tru'/" /etc/config/routeguard
  if routeguard apply; then
    echo 'Invalid boolean accepted' >&2
    exit 1
  fi
  verified
  cp /src/tests/integration/fixtures/manual.uci /etc/config/routeguard
}

check_rollback_and_restore() {
  sed -i '/list ip/d' /etc/config/routeguard
  printf "\tlist ip '10.2.0.4'\n" >> /etc/config/routeguard
  routeguard apply
  blocked ping -c 1 -W 1 10.2.0.4
  allowed ping -c 1 -W 1 10.2.0.2
  routeguard rollback
  routeguard refresh
  nft delete table inet routeguard
  routeguard restore
  verified
  blocked ping -c 1 -W 1 10.2.0.2
  allowed ping -c 1 -W 1 10.2.0.4
}

check_restore_without_snapshot() {
  # Reconstruct from explicit UCI when a snapshot is absent; startup must not
  # silently treat an enabled configuration as a blank installation.
  cp /src/tests/integration/fixtures/manual.uci /etc/config/routeguard
  rm /etc/routeguard/state.json
  nft delete table inet routeguard
  routeguard recover
  verified
}

check_interrupted_worker() {
  # Queue acceptance, result identity and recovery after an actual worker death.
  python3 /src/tests/integration/silent-dns.py &
  dns_pid=$!
  trap 'kill "$dns_pid" 2>/dev/null || true' EXIT
  attempt=0
  while [ ! -f /tmp/silent-dns-ready ]; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 50 ] || exit 1
    sleep 0.1
  done
  cp /src/tests/integration/fixtures/slow.uci /etc/config/routeguard
  python3 - << 'PY'
import json, os, signal, subprocess, time
reply = json.loads(subprocess.check_output(['routeguard', 'queue', 'apply']))
assert reply['queued'] and reply['id'], reply
job = json.loads(subprocess.check_output(['routeguard', 'job', reply['id']]))
assert job['id'] == reply['id'] and job['state'] == 'running', job
os.kill(job['pid'], signal.SIGKILL)
for _ in range(50):
    status = json.loads(subprocess.check_output(['routeguard', 'status']))
    if not status['busy']:
        assert status['job']['state'] == 'interrupted', status
        break
    time.sleep(.1)
else:
    raise AssertionError('dead worker left the engine busy')
PY
  cp /src/tests/integration/fixtures/manual.uci /etc/config/routeguard
  routeguard apply
  verified
}

check_journal_replay() {
  # The journal is replayed without resolving domains or reading pending UCI.
  python3 - << 'PY'
import json, pathlib
path = pathlib.Path('/etc/routeguard/state.json')
state = json.loads(path.read_text())
state['active']['plan']['targets'] = ['10.2.0.4/32']
state['generation'] += 1
path.with_name('state.json.pending').write_text(json.dumps({'version': 1, 'target': state}))
PY
  routeguard restore
  verified
  blocked ping -c 1 -W 1 10.2.0.4
  test ! -f /etc/routeguard/state.json.pending
}

check_stop() {
  routeguard stop
  if nft list table inet routeguard > /dev/null 2>&1; then exit 1; fi
  nft list table inet fw4 > /dev/null
}

main() {
  require_test_container
  install_test_tools
  configure_network

  check_blank_configuration
  check_wan_filtering
  python3 /src/tests/integration/diagnostics.py
  check_refresh_counters
  check_firewall_repair
  check_invalid_configuration
  check_rollback_and_restore
  check_restore_without_snapshot
  check_interrupted_worker
  check_journal_replay
  check_stop

  printf '%s\n' 'PASS: IPv4/IPv6, output/forward, multiple WANs, exceptions, drift repair, counters, failure retention, rollback, restore and stop.'
}

main "$@"
