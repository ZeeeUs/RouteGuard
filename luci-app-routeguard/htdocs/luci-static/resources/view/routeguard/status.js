"use strict";
"require view";
"require form";
"require routeguard.status as status";

return view.extend({
  render: async function () {
    const map = new form.Map("routeguard");
    await map.load();
    return status.create(map.readonly);
  },
  handleSave: null,
  handleSaveApply: null,
  handleReset: null,
});
