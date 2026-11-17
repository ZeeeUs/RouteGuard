import { stdin } from 'fs';
import { connect } from 'ubus';

// Keep the authenticated LuCI session out of process arguments and logs.
const request = json(stdin.read(4096));
const session = request?.ubus_rpc_session;

if (type(session) != 'string' || !match(session, /^[a-f0-9]{32}$/))
	exit(1);

const bus = connect();
const staged = bus?.call('uci', 'get', {
	config: 'routeguard',
	ubus_rpc_session: session,
});
if (!staged)
	exit(1);

printf('%J', staged);
