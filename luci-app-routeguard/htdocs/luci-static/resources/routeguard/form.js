"use strict";
"require baseclass";
"require form";

function createSettingsContent(section) {
  let o = section.option(form.Flag, "enabled", _("Enable RouteGuard"));
  o.rmempty = true;

  o = section.option(
    form.DynamicList,
    "wan_device",
    _("Linux WAN devices"),
    _("Use actual outgoing devices such as pppoe-wan. Add each uplink, including IPv6 uplinks."),
  );
  o.validate = function (section_id, value) {
    if (!value || /^[A-Za-z0-9_][A-Za-z0-9_.:-]{0,14}$/.test(value)) {
      return true;
    }
    return _("Enter a Linux device name.");
  };

  o = section.option(
    form.Value,
    "dns",
    _("Resolver IP"),
    _("Required for domains and remote lists. Leave empty for IP-only rules."),
  );
  o.datatype = "ipaddr(1)";

  o = section.option(form.ListValue, "ipv6", _("IPv6 mode"));
  o.value("", _("Select an IPv6 mode"));
  o.value("off", _("Off (IPv6 traffic is unprotected)"));
  o.value("selective", _("Selective (AAAA and manual IPv6 targets)"));

  o = section.option(
    form.Value,
    "update_minute",
    _("Hourly update minute"),
    _("Enter a minute from 0 to 59, or leave empty to update manually."),
  );
  o.datatype = "and(uinteger,range(0,59))";

  o = section.option(
    form.DynamicList,
    "exempt_ip",
    _("Tunnel endpoint exceptions"),
    _(
      "Single IPs that can bypass RouteGuard on WAN, for example the address of the tunnel server.",
    ),
  );
  o.datatype = "ipaddr(1)";
}

function createServiceContent(section) {
  let o = section.option(form.Flag, "enabled", _("Enabled"));
  o.rmempty = true;
  section.option(form.Value, "label", _("Name"));

  section.option(
    form.DynamicList,
    "url",
    _("Remote HTTPS domain lists"),
    _(
      "Use one hostname per line. Optional domain: and full: prefixes are accepted. Tagged entries and include directives are skipped.",
    ),
  );

  section.option(
    form.DynamicList,
    "domain",
    _("Manual domains"),
    _("Enter exact hostnames. Add subdomains individually."),
  );

  o = section.option(form.DynamicList, "ip", _("Manual IP addresses and CIDRs"));
  o.datatype = "ipaddr";
}

return baseclass.extend({ settings: createSettingsContent, services: createServiceContent });
