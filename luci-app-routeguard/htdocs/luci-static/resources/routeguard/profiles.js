"use strict";
"require baseclass";
"require form";
"require ui";

const ProfileSection = form.TypedSection.extend({
  renderContents: function (ids, nodes) {
    const titled = nodes.map((node, index) =>
      E("div", {}, [
        E("h3", {}, [
          this.map.data.get(this.map.config, ids[index], "label") || _("Unnamed profile"),
        ]),
        node,
      ]),
    );
    return this.super("renderContents", [ids, titled]);
  },

  renderSectionAdd: function () {
    const name = E("input", {
      type: "text",
      class: "cbi-input-text",
      placeholder: _("New profile name"),
      "aria-label": _("New profile name"),
      maxlength: 128,
      disabled: this.map.readonly,
    });
    const add = E(
      "button",
      {
        type: "button",
        class: "cbi-button cbi-button-add",
        disabled: true,
        click: ui.createHandlerFn(this, async () => {
          const label = name.value.trim();
          if (!label || this.map.readonly) return;

          await this.map.save(null, true);
          const id = this.map.data.add(this.map.config, this.sectiontype);
          this.map.data.set(this.map.config, id, "label", label);
          return this.map.save(null, true);
        }),
      },
      [_("Create profile")],
    );
    name.addEventListener("input", () => {
      add.disabled = this.map.readonly || !name.value.trim();
    });
    name.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        if (!add.disabled) add.click();
      }
    });
    return E("div", { class: "cbi-section-create" }, [name, " ", add]);
  },
});

return baseclass.extend({ ProfileSection });
