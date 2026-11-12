"use strict";
"require view";
"require form";
"require ui";
"require routeguard.form as fields";
"require routeguard.profiles as profiles";
"require routeguard.actions as actions";

return view.extend({
  render: async function () {
    const map = new form.Map(
      "routeguard",
      _("RouteGuard"),
      _(
        "Blocks known destination addresses on the selected WAN devices. Unlisted addresses, DNS variations and shared CDN IPs affect coverage. Review IPv6 separately.",
      ),
    );
    this.map = map;

    fields.settings(map.section(form.NamedSection, "main", "settings", _("Settings")));

    const services = map.section(profiles.ProfileSection, "service", _("Protection profiles"));
    services.anonymous = true;
    services.addremove = true;
    fields.services(services);

    const formNode = await map.render();
    return formNode;
  },

  handleSaveApply: async function () {
    try {
      await actions.saveAndApply(this.map);
    } catch (error) {
      ui.addNotification(null, E("p", {}, [error.message]), "error");
      throw error;
    }
  },
});
