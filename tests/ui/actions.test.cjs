const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const source = fs.readFileSync(
  path.join(
    __dirname,
    "../../luci-app-routeguard/htdocs/luci-static/resources/routeguard/actions.js",
  ),
  "utf8",
);

function fixture(overrides = {}) {
  const events = [];
  const handlers = {
    commit: async (config) => {
      assert.equal(config, "routeguard");
      events.push("commit");
    },
    apply: async () => {
      events.push("queue");
      return { queued: true, id: "request-1" };
    },
    job: async (id) => {
      events.push("result");
      return { id, state: "ok" };
    },
    ...overrides,
  };
  const actions = new Function("baseclass", "rpc", "ui", "_", source)(
    { extend: (object) => object },
    {
      declare:
        ({ method }) =>
        (...args) =>
          handlers[method](...args),
    },
    { changes: { init: async () => {} } },
    (value) => value,
  );
  const map = { save: async () => events.push("save") };
  return { actions, map, events };
}

test("Save & Apply saves, commits and waits for its own operation", async () => {
  const { actions, map, events } = fixture();
  const result = await actions.saveAndApply(map);
  assert.equal(result.state, "ok");
  assert.deepEqual(events, ["save", "commit", "queue", "result"]);
});

test("form validation failure never commits or applies", async () => {
  const { actions, events } = fixture();
  await assert.rejects(
    actions.saveAndApply({
      save: async () => {
        throw new Error("validation failed");
      },
    }),
    /validation failed/,
  );
  assert.deepEqual(events, []);
});

test("commit failure never starts firewall work", async () => {
  const { actions, map, events } = fixture({
    commit: async () => {
      throw new Error("commit failed");
    },
  });
  await assert.rejects(actions.saveAndApply(map), /commit failed/);
  assert.deepEqual(events, ["save"]);
});

test("startup refusal and interrupted jobs are shown as failures", async () => {
  const refused = fixture({ apply: async () => ({ queued: false, error: "RouteGuard is busy" }) });
  await assert.rejects(refused.actions.run("apply"), /busy/);

  const interrupted = fixture({
    job: async (id) => ({ id, state: "interrupted", message: "worker stopped" }),
  });
  await assert.rejects(interrupted.actions.run("apply"), /worker stopped/);
});

test("a different operation cannot satisfy this request", async () => {
  const { actions } = fixture({ job: async () => ({ id: "another-2", state: "ok" }) });
  await assert.rejects(actions.run("apply"), /unavailable/);
});
