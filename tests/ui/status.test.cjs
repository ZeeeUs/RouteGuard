const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const source = fs.readFileSync(
  path.join(
    __dirname,
    "../../luci-app-routeguard/htdocs/luci-static/resources/routeguard/status.js",
  ),
  "utf8",
);
const fixture = JSON.parse(fs.readFileSync(path.join(__dirname, "testdata/status.json"), "utf8"));

String.prototype.format = function (...args) {
  let index = 0;
  return this.replace(/%[sd]/g, () => String(args[index++]));
};
function node(tag, attrs, children = []) {
  return {
    tag,
    attrs,
    children,
    appendChild(child) {
      this.children.push(child);
    },
    replaceChildren() {
      this.children = [];
    },
  };
}
const render = new Function(
  "E",
  "_",
  source.slice(0, source.indexOf("const getStatus")) + "\nreturn showStatus;",
)(node, (text) => text);
function text(data) {
  const root = node("div", {});
  render(root, data);
  function flatten(value) {
    if (Array.isArray(value)) return value.map(flatten).join(" ");
    if (value && typeof value === "object") return flatten(value.children);
    return String(value);
  }
  return flatten(root);
}

test("a failed refresh does not hide verified rules or the IPv6 limitation", () => {
  const output = text(fixture);
  assert.match(output, /Firewall rules are verified/);
  assert.match(output, /IPv6 traffic is not protected/);
  assert.match(output, /Download failed/);
  assert.match(output, /LAN clients · IPv4 6 384/);
  assert.match(output, /Every hour at minute 0/);
});

test("paused and broken schedules are never presented as operational", () => {
  assert.match(text({ ...fixture, paused: true }), /Paused after rollback/);
  assert.match(
    text({ ...fixture, schedule: { ...fixture.schedule, running: false } }),
    /not operational/,
  );
});

test("missing status never claims protection and empty data is renderable", () => {
  assert.match(text(null), /could not be verified/);
  assert.match(text({ loaded: false }), /Protection is not verified/);
  assert.match(text({ loaded: false }), /No profiles in the active rules/);
});
