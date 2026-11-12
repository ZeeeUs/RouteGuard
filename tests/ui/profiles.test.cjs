const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const source = fs.readFileSync(
  path.join(
    __dirname,
    "../../luci-app-routeguard/htdocs/luci-static/resources/routeguard/profiles.js",
  ),
  "utf8",
);

function fixture() {
  const events = [];
  const values = {};
  const profile = new Function("form", "ui", "E", "_", source)(
    { TypedSection: { extend: (value) => value } },
    { createHandlerFn: (context, fn) => fn.bind(context) },
    (tag, attrs, children) => ({
      tag,
      ...attrs,
      children,
      value: "",
      listeners: {},
      addEventListener(event, fn) {
        this.listeners[event] = fn;
      },
    }),
    (value) => value,
  );
  profile.sectiontype = "service";
  profile.map = {
    config: "routeguard",
    save: async () => events.push("save"),
    data: {
      add: () => {
        events.push("add");
        return "new_profile";
      },
      set: (config, id, key, value) => {
        values[key] = value;
      },
      get: () => "Existing profile",
    },
  };
  return { profile, events, values };
}

test("creation requires a name, saves existing edits and sets the label before the second save", async () => {
  const { profile, events, values } = fixture();
  const [name, , button] = profile.renderSectionAdd().children;
  name.value = "   ";
  name.listeners.input();
  assert.equal(button.disabled, true);
  await button.click();
  assert.deepEqual(events, []);
  name.value = " My profile ";
  name.listeners.input();
  assert.equal(button.disabled, false);
  await button.click();
  assert.deepEqual(events, ["save", "add", "save"]);
  assert.equal(values.label, "My profile");
});

test("invalid existing settings cannot create a partially configured profile", async () => {
  const { profile, events } = fixture();
  profile.map.save = async () => {
    throw new Error("invalid settings");
  };
  const [name, , button] = profile.renderSectionAdd().children;
  name.value = "New profile";
  await assert.rejects(button.click(), /invalid settings/);
  assert.deepEqual(events, []);
});

test("existing anonymous profiles keep their label as a text heading", () => {
  const { profile } = fixture();
  profile.super = (method, args) => args;
  const [, nodes] = profile.renderContents(
    ["cfg025996"],
    [{ tag: "fragment", children: ["fields"] }],
  );
  assert.equal(nodes[0].children[0].tag, "h3");
  assert.deepEqual(nodes[0].children[0].children, ["Existing profile"]);
});
