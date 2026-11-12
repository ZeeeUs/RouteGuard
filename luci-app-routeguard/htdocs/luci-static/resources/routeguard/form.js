"use strict";
"require baseclass";
"require form";
"require tools.widgets as widgets";

function createSettingsContent(section) {
  let o = section.option(form.Flag, "enabled", _("Block direct connections"));
  o.rmempty = true;

  o = section.option(
    widgets.DeviceSelect,
    "wan_device",
    _("Internet uplink devices"),
    _(
      "Select the devices used to connect to the Internet, for example wan or pppoe-wan. You can select several uplinks or enter a device name manually if it is temporarily absent.",
    ),
  );
  o.multiple = true;
  o.noaliases = true;
  o.noinactive = false;
  o.nocreate = false;
  o.rmempty = true;
  o.validate = function (section_id, value) {
    const devices = Array.isArray(value) ? value : value ? [value] : [];
    if (devices.every((device) => /^[A-Za-z0-9_][A-Za-z0-9_.:-]{0,14}$/.test(device))) {
      return true;
    }
    return _("Enter an outgoing device name, for example wan or pppoe-wan.");
  };

  o = section.option(
    form.Value,
    "dns",
    _("DNS server"),
    _(
      "Select a DNS server or enter a custom IPv4 or IPv6 address. Used to find real IP addresses for domains. Required for domain lists; leave empty for IP-only profiles.",
    ),
  );
  o.value("1.1.1.1", "Cloudflare (1.1.1.1)");
  o.value("8.8.8.8", "Google (8.8.8.8)");
  o.value("9.9.9.9", "Quad9 (9.9.9.9)");
  o.datatype = "ipaddr(1)";
  o.rmempty = true;

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
  let o = section.option(form.Flag, "enabled", _("Protect this profile"));
  o.rmempty = true;

  section.option(
    form.DynamicList,
    "url",
    _("Remote HTTPS domain lists"),
    _(
      "Use a direct HTTPS link to a text file. On GitHub, copy the Raw link. Use one hostname per line. Optional domain: and full: prefixes are accepted. Tagged entries and include directives are skipped.",
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
