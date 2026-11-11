"use strict";
"require baseclass";
"require rpc";
"require poll";
"require ui";
"require routeguard.actions as actions";

function addStatusLine(parent, label, value) {
  parent.appendChild(E("div", {}, [E("strong", {}, [label]), " ", String(value)]));
}

function showStatus(parent, data) {
  parent.replaceChildren();
  if (!data || data.error || typeof data.loaded !== "boolean") {
    addStatusLine(parent, _("Status"), data?.error || _("Unavailable"));
    return;
  }

  addStatusLine(parent, _("Rules in kernel"), data.loaded ? _("Verified") : _("Not verified"));
  addStatusLine(parent, _("Last rules change"), data.updated || "—");
  addStatusLine(parent, _("Active WAN devices"), data.devices.join(", ") || "—");
  addStatusLine(
    parent,
    _("IPv4 default route devices"),
    data.default_route_devices["-4"].join(", ") || "—",
  );
  addStatusLine(
    parent,
    _("IPv6 default route devices"),
    data.default_route_devices["-6"].join(", ") || "—",
  );
  addStatusLine(
    parent,
    _("IPv6"),
    data.loaded && data.ipv6 === "selective"
      ? _("Selective filtering of known addresses")
      : _("Unprotected by RouteGuard"),
  );
  if (data.reason) {
    addStatusLine(parent, _("Protection"), data.reason);
  }
  if (data.recovery_pending) {
    addStatusLine(
      parent,
      _("Recovery"),
      _("An interrupted change needs recovery. Use Save & Apply."),
    );
  }
  for (const warning of data.warnings || []) {
    addStatusLine(parent, _("Warning"), warning);
  }
  if (data.schedule?.minute) {
    addStatusLine(parent, _("Scheduled updates"), data.schedule.error || _("Operational"));
  }
  if (data.offload) {
    addStatusLine(
      parent,
      _("Warning"),
      _("Flow offloading is configured and may bypass filtering."),
    );
  }
  if (data.missing_devices.length) {
    addStatusLine(parent, _("Missing devices"), data.missing_devices.join(", "));
  }
  if (data.paused) {
    addStatusLine(parent, _("Updates"), _("Paused after rollback. Save & Apply to resume."));
  }
  if (data.job && data.job.action) {
    const job = data.job;
    addStatusLine(
      parent,
      _("Last operation"),
      [job.action, job.state, job.message].filter(Boolean).join(" / "),
    );
  }

  for (const service of data.services) {
    addStatusLine(
      parent,
      service.label,
      _("%d domains, %d IPv4 entries, %d IPv6 entries, %d skipped list records").format(
        service.domains,
        service.ipv4,
        service.ipv6,
        service.skipped,
      ),
    );
  }

  for (const name of Object.keys(data.counters).sort()) {
    const counter = data.counters[name];
    addStatusLine(parent, name, _("%s packets / %s bytes").format(counter.packets, counter.bytes));
  }
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
      E("h3", {}, [_("Live status")]),
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
