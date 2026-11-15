"use strict";
"require view";
"require form";
"require rpc";
"require ui";
"require routeguard.actions as actions";

const getStatus = rpc.declare({ object: "luci.routeguard", method: "status" });

function renderReport(report) {
  const outcomes = {
    drop_observed: {
      label: _("Drop observed"),
      icon: "✓",
      color: "#198754",
      background: "rgba(25, 135, 84, 0.12)",
    },
    reachable: {
      label: _("Connection succeeded"),
      icon: "✕",
      color: "#c62828",
      background: "rgba(198, 40, 40, 0.12)",
    },
    inconclusive: {
      label: _("Inconclusive"),
      icon: "?",
      color: "#855b00",
      background: "rgba(185, 128, 0, 0.12)",
    },
    skipped: {
      label: _("Not tested"),
      icon: "—",
      color: "#59636e",
      background: "transparent",
    },
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
    const outcome = outcomes[check.result] || outcomes.inconclusive;
    const result = E(
      "span",
      {
        style: `display: inline-block; padding: 0.2em 0.6em; border-radius: 0.3em; color: #fff; background-color: ${outcome.color}; font-weight: 600`,
      },
      [
        E("span", { "aria-hidden": "true" }, [outcome.icon]),
        " ",
        outcomes[check.result] ? outcome.label : check.result,
      ],
    );
    const cells = [
      check.profile,
      check.device || "—",
      check.address || "—",
      result,
      String(check.packets),
      check.detail,
    ];
    return E(
      "tr",
      { class: "tr", style: `background-color: ${outcome.background}` },
      cells.map((value) => E("td", { class: "td" }, [value])),
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
