"use strict";
"require view";
"require form";
"require rpc";
"require ui";
"require routeguard.actions as actions";

const getStatus = rpc.declare({ object: "luci.routeguard", method: "status" });

function renderReport(report) {
  const labels = {
    drop_observed: _("Drop observed"),
    reachable: _("Connection succeeded"),
    inconclusive: _("Inconclusive"),
    skipped: _("Not tested"),
  };
  const headers = [
    _("Profile"),
    _("Uplink"),
    _("Address · TCP 443"),
    _("Result"),
    _("Dropped packets during probe"),
    _("Details"),
  ];
  const rows = report.checks.map((check) => {
    const cells = [
      check.profile,
      check.device || "—",
      check.address || "—",
      labels[check.result] || check.result,
      check.packets,
      check.detail,
    ];
    return E(
      "tr",
      { class: "tr" },
      cells.map((value) => E("td", { class: "td" }, [String(value)])),
    );
  });
  return E("div", {}, [
    E("p", {}, [_("Checked at"), " ", new Date(report.time).toLocaleString()]),
    E("div", { style: "overflow-x: auto" }, [
      E("table", { class: "table" }, [
        E("thead", {}, [
          E(
            "tr",
            { class: "tr" },
            headers.map((label) => E("th", { class: "th" }, [label])),
          ),
        ]),
        E("tbody", {}, rows),
      ]),
    ]),
  ]);
}

return view.extend({
  render: async function () {
    const map = new form.Map("routeguard");
    await map.load();
    const results = E("div", { "aria-live": "polite" });
    const button = E(
      "button",
      {
        type: "button",
        class: "cbi-button cbi-button-action",
        disabled: map.readonly,
        click: ui.createHandlerFn(this, async () => {
          button.disabled = true;
          results.replaceChildren(
            E("p", {}, [_("Checking profiles. This may take up to 90 seconds…")]),
          );
          try {
            const operation = await actions.run("diagnose");
            if (!operation.diagnostics)
              throw new Error(_("Diagnostic result is unavailable. Update the RouteGuard engine."));
            results.replaceChildren(renderReport(operation.diagnostics));
          } catch (error) {
            results.replaceChildren(E("p", { class: "alert-message warning" }, [error.message]));
          } finally {
            button.disabled = map.readonly;
          }
        }),
      },
      [_("Check all profiles")],
    );
    try {
      const data = await getStatus();
      if (data?.job?.diagnostics) results.appendChild(renderReport(data.job.diagnostics));
    } catch (_) {
      /* The check button reports connection errors when used. */
    }
    return E("div", { class: "cbi-map" }, [
      E("h2", {}, [_("Profile diagnostics")]),
      E("p", {}, [
        _(
          "Checks applied profiles using real TCP connections from this router, bound to each selected Internet uplink. Save & Apply changes before testing. The tunnel and firewall rules are not changed.",
        ),
      ]),
      E("p", { class: "alert-message warning" }, [
        _(
          "This is a sampled TCP 443 test: at most one saved host address per IP family and profile. It does not test LAN forwarding, all addresses, UDP, or tunnel availability. A timeout alone is not proof of protection. Counter increases can include background traffic.",
        ),
      ]),
      E("p", {}, [
        _(
          "Profiles saved by older versions need an address refresh before testing. CIDR-only and fully exempt profiles may have no testable host address.",
        ),
      ]),
      button,
      results,
    ]);
  },
  handleSave: null,
  handleSaveApply: null,
  handleReset: null,
});
