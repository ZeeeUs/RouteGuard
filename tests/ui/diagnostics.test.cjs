const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const resources = path.join(__dirname, "../../luci-app-routeguard/htdocs/luci-static/resources");
const source = fs.readFileSync(path.join(resources, "view/routeguard/diagnostics.js"), "utf8");
const report = JSON.parse(
  fs.readFileSync(path.join(__dirname, "testdata/diagnostics.json"), "utf8"),
);
function node(tag, attrs, children = []) {
  return {
    tag,
    ...attrs,
    children,
    appendChild(child) {
      this.children.push(child);
    },
    replaceChildren(...items) {
      this.children = items;
    },
  };
}
function flatten(value) {
  if (Array.isArray(value)) return value.map(flatten).join(" ");
  if (value && typeof value === "object") return flatten(value.children);
  return String(value);
}
async function fixture(run, readonly = false) {
  const view = new Function("view", "form", "rpc", "ui", "actions", "E", "_", source)(
    { extend: (value) => value },
    {
      Map: class {
        constructor() {
          this.readonly = readonly;
        }
        async load() {}
      },
    },
    { declare: () => async () => ({}) },
    { createHandlerFn: (context, fn) => fn.bind(context) },
    { run },
    node,
    (value) => value,
  );
  const root = await view.render();
  return { view, root, button: root.children.find((item) => item.tag === "button") };
}
test("one click checks all profiles and presents uncertainty separately from observed drops", async () => {
  let calls = 0;
  const { view, root, button } = await fixture(async (action) => {
    assert.equal(action, "diagnose");
    calls++;
    return { diagnostics: report };
  });
  assert.equal(view.handleSaveApply, null);
  await button.click();
  assert.equal(calls, 1);
  assert.equal(button.disabled, false);
  const output = flatten(root);
  for (const label of ["Drop observed", "Connection succeeded", "Inconclusive", "Not tested"])
    assert.ok(output.includes(label), label);
});
test("diagnostic RPC failure is visible and permits retry", async () => {
  const { root, button } = await fixture(async () => {
    throw new Error("RouteGuard is busy");
  });
  await button.click();
  assert.match(flatten(root), /RouteGuard is busy/);
  assert.equal(button.disabled, false);
});
test("read-only users cannot start probes", async () => {
  const { button } = await fixture(async () => {}, true);
  assert.equal(button.disabled, true);
});
test("every RouteGuard tab has a syntactically valid view", () => {
  const menu = JSON.parse(
    fs.readFileSync(
      path.join(
        __dirname,
        "../../luci-app-routeguard/root/usr/share/luci/menu.d/luci-app-routeguard.json",
      ),
      "utf8",
    ),
  );
  assert.equal(menu["admin/services/routeguard"].action.type, "firstchild");
  for (const name of ["settings", "status", "diagnostics"]) {
    const entry = menu[`admin/services/routeguard/${name}`];
    const code = fs.readFileSync(path.join(resources, "view", entry.action.path + ".js"), "utf8");
    assert.doesNotThrow(() => new Function(code));
  }
});
