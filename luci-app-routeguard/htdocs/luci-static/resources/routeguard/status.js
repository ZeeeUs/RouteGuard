"use strict";
"require baseclass";
"require rpc";
"require poll";
"require ui";
"require routeguard.actions as actions";

function table(headers, rows) {
  return E("div", { style: "overflow-x: auto" }, [
    E("table", { class: "table" }, [
      E("thead", {}, [
        E(
          "tr",
          { class: "tr" },
          headers.map((title) => E("th", { class: "th" }, [title])),
        ),
      ]),
      E(
        "tbody",
        {},
        rows.map((row) =>
          E(
            "tr",
            { class: "tr" },
            row.map((value) => E("td", { class: "td" }, [String(value ?? "—")])),
          ),
        ),
      ),
    ]),
  ]);
}

function group(parent, title, rows) {
  parent.appendChild(E("h4", {}, [title]));
  parent.appendChild(table([_("Item"), _("State")], rows));
}

function timestamp(value) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function showStatus(parent, data) {
  parent.replaceChildren();
  if (!data || data.error || typeof data.loaded !== "boolean") {
    parent.appendChild(
      E("p", { class: "alert-message warning" }, [
        data?.error || _("Status unavailable. Protection could not be verified."),
      ]),
    );
    return;
  }

  parent.appendChild(
    E("h4", {}, [data.loaded ? _("Firewall rules are verified") : _("Protection is not verified")]),
  );
  parent.appendChild(
    E("p", {}, [
      _(
        "Active rules are shown below. Unsaved form changes are not reflected here. Only known IP addresses are covered; shared CDN addresses may affect other sites.",
      ),
    ]),
  );
  const warnings = [...(data.warnings || [])];
  if (data.reason) warnings.push(data.reason);
  if (data.recovery_pending)
    warnings.push(_("An interrupted change needs recovery. Use Save & Apply."));
  if (data.offload)
    warnings.push(_("Flow offloading may bypass filtering. Disable it in firewall settings."));
  if (data.missing_devices?.length)
    warnings.push(_("Unavailable uplinks: %s").format(data.missing_devices.join(", ")));
  if (data.ipv6 !== "selective" || !data.loaded)
    warnings.push(_("IPv6 traffic is not protected by verified RouteGuard rules."));
  for (const warning of warnings)
    parent.appendChild(E("p", { class: "alert-message warning" }, [warning]));

  group(parent, _("Active protection"), [
    [_("Firewall rules"), data.loaded ? _("Verified") : _("Not verified")],
    [_("Protected uplinks"), data.devices?.join(", ") || "—"],
    [
      _("IPv4 Internet route"),
      data.default_route_devices?.["-4"]?.join(", ") || _("No default route"),
    ],
    [
      _("IPv6 Internet route"),
      data.default_route_devices?.["-6"]?.join(", ") || _("No default route"),
    ],
    [
      _("IPv6 protection"),
      data.loaded && data.ipv6 === "selective"
        ? _("Known IPv6 addresses only")
        : _("Not protected"),
    ],
    [_("Rules last changed"), timestamp(data.updated)],
  ]);

  const schedule = data.schedule || {};
  const scheduled = schedule.minute !== undefined && schedule.minute !== "";
  let updates = _("Manual updates");
  if (data.paused) updates = _("Paused after rollback. Save & Apply to resume.");
  else if (scheduled)
    updates =
      schedule.error ||
      (schedule.installed && schedule.running && schedule.enabled
        ? _("Every hour at minute %s").format(schedule.minute)
        : _("Scheduled updates are not operational"));
  const job = data.job || {};
  const operations = {
    apply: _("Apply settings"),
    refresh: _("Update addresses"),
    restore: _("Restore saved rules"),
    rollback: _("Roll back rules"),
    stop: _("Stop protection"),
    recover: _("Recover interrupted change"),
  };
  const results = {
    ok: _("Successful"),
    error: _("Failed"),
    interrupted: _("Interrupted"),
    running: _("Running"),
    queued: _("Queued"),
  };
  group(parent, _("Address updates"), [
    [_("Schedule"), updates],
    [_("Current operation"), data.busy ? _("In progress") : _("Idle")],
    [_("Last operation"), operations[job.action] || job.action || "—"],
    [_("Result"), results[job.state] || job.state || "—"],
    [_("Details"), job.message || "—"],
    [_("Operation time"), timestamp(job.time)],
  ]);

  parent.appendChild(E("h4", {}, [_("Active profiles")]));
  if (data.services?.length) {
    parent.appendChild(
      table(
        [
          _("Profile"),
          _("Domains"),
          _("IPv4 entries"),
          _("IPv6 entries"),
          _("Skipped list records"),
        ],
        data.services.map((profile) => [
          profile.label || profile.id,
          profile.domains,
          profile.ipv4,
          profile.ipv6,
          profile.skipped,
        ]),
      ),
    );
  } else
    parent.appendChild(
      E("p", {}, [_("No profiles in the active rules. Configure a profile and use Save & Apply.")]),
    );

  parent.appendChild(E("h4", {}, [_("Blocked traffic")]));
  const counters = data.counters || {};
  const names = {
    forward4: _("LAN clients · IPv4"),
    output4: _("Router · IPv4"),
    forward6: _("LAN clients · IPv6"),
    output6: _("Router · IPv6"),
  };
  parent.appendChild(
    table(
      [_("Traffic source"), _("Dropped packets"), _("Dropped bytes")],
      Object.keys(counters)
        .sort()
        .map((name) => [names[name] || name, counters[name].packets, counters[name].bytes]),
    ),
  );
  parent.appendChild(
    E("p", {}, [
      _(
        "Counters count packets, not connections. They may reset when rules are restored or the router restarts. Zero does not mean protection is inactive.",
      ),
    ]),
  );
}

const getStatus = rpc.declare({ object: "luci.routeguard", method: "status" });

return baseclass.extend({
  create: function (readonly) {
    const panel = E("div", {});
    const buttons = [];
    let localBusy = false;

    const button = (label, action) => {
      const node = E(
        "button",
        {
          class: "btn cbi-button-action",
          type: "button",
          disabled: readonly,
          click: async () => {
            localBusy = true;
            buttons.forEach((item) => {
              item.disabled = true;
            });
            try {
              await actions.run(action);
            } catch (error) {
              ui.addNotification(null, E("p", {}, [error.message]), "error");
            } finally {
              localBusy = false;
              await update();
            }
          },
        },
        label,
      );
      buttons.push(node);
      return node;
    };

    const refresh = button(_("Refresh active addresses"), "refresh");
    const rollback = button(_("Rollback rules"), "rollback");
    rollback.disabled = true;

    const update = async () => {
      try {
        const data = await getStatus();
        showStatus(panel, data);
        const unavailable = !data || data.error || typeof data.loaded !== "boolean";
        const busy = readonly || localBusy || data?.busy || unavailable;
        refresh.disabled = busy || !data?.table_present || data?.paused;
        rollback.disabled = busy || !data?.rollback_available;
      } catch (error) {
        showStatus(panel, { error: error.message });
        buttons.forEach((item) => {
          item.disabled = true;
        });
      }
    };

    poll.add(update, 10);
    update();
    return E("div", {}, [
      E("h3", {}, [_("Protection status")]),
      panel,
      E("p", {}, [
        _(
          "Save & Apply saves settings and waits for the firewall result. Refresh only updates the active configuration. Logs: logread -e routeguard.",
        ),
      ]),
      refresh,
      " ",
      rollback,
    ]);
  },
});
