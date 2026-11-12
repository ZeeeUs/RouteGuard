"use strict";
"require baseclass";
"require rpc";
"require ui";

const commit = rpc.declare({ object: "uci", method: "commit", params: ["config"], reject: true });
const getJob = rpc.declare({ object: "luci.routeguard", method: "job", params: ["id"] });
const commands = Object.fromEntries(
  ["apply", "refresh", "rollback"].map((method) => [
    method,
    rpc.declare({ object: "luci.routeguard", method }),
  ]),
);

async function waitForJob(id) {
  const deadline = Date.now() + 180000;

  while (Date.now() < deadline) {
    const job = await getJob(id);
    if (job?.error) {
      throw new Error(job.error);
    }
    if (!job || job.id !== id) {
      throw new Error(_("Operation result is unavailable. Check live status."));
    }
    if (job.state === "ok") {
      return job;
    }
    if (job.state === "error" || job.state === "interrupted") {
      throw new Error(job.message || _("The operation failed."));
    }

    await new Promise((resolve) => setTimeout(resolve, 1000));
  }

  throw new Error(_("The operation has not finished yet. Check live status."));
}

return baseclass.extend({
  run: async function (action) {
    if (!commands[action]) {
      throw new Error(_("Unknown operation."));
    }

    const reply = await commands[action]();
    if (!reply?.queued || !reply.id) {
      throw new Error(reply?.error || _("The request was rejected."));
    }

    return waitForJob(reply.id);
  },

  saveAndApply: async function (map) {
    await map.save(null, true);
    await commit("routeguard");

    // Committing can change the IDs of anonymous UCI sections.
    map.data.unload("routeguard");
    await map.render();

    // Commit only this package. Unrelated pending LuCI changes stay pending.
    const [, operation] = await Promise.all([ui.changes.init().catch(() => {}), this.run("apply")]);
    return operation;
  },
});
