// SPDX-License-Identifier: MIT
import assert from "node:assert/strict";
import { mkdtemp, writeFile, rm, symlink } from "node:fs/promises";
import test from "node:test";
import { waitForEndpoint } from "./readiness.mjs";

async function fixture(t) {
  const directory = await mkdtemp("/tmp/sb-ready-");
  t.after(() => rm(directory, { recursive: true, force: true }));
  return directory;
}

function ownedWait(t, directory, lifetime = new AbortController()) {
  const ready = waitForEndpoint(directory, lifetime.signal);
  const settled = ready.then(() => {}, () => {});
  t.after(async () => {
    lifetime.abort();
    await settled;
  });
  return ready;
}

for (const first of [true, false]) {
  test("empty ready marker " + (first ? "before" : "after") + " watcher", { timeout: 5000 }, async (t) => {
    const dir = await fixture(t);
    const lifetime = new AbortController();
    if (first) await writeFile(dir + "/actions.ready", "", { flag: "wx", mode: 0o600 });
    const ready = ownedWait(t, dir, lifetime);
    if (!first) await writeFile(dir + "/actions.ready", "", { flag: "wx", mode: 0o600 });
    assert.equal(await ready, dir + "/actions.sock");
    lifetime.abort();
  });
}

test("disposal cancels missing-marker wait and joins watcher", { timeout: 5000 }, async (t) => {
  const dir = await fixture(t);
  const lifetime = new AbortController();
  const ready = ownedWait(t, dir, lifetime);
  const rejected = assert.rejects(ready, /native disposed/);
  lifetime.abort(new Error("native disposed"));
  await rejected;
  await writeFile(dir + "/actions.ready", ""); // no resumed work/reconnect
});

for (const link of [false, true]) {
  test("reject " + (link ? "symlink" : "nonempty") + " marker", async (t) => {
    const dir = await fixture(t);
    if (link) {
      await writeFile(dir + "/other", "");
      await symlink("other", dir + "/actions.ready");
    } else await writeFile(dir + "/actions.ready", "stale data");
    await assert.rejects(ownedWait(t, dir), /invalid.*marker/);
  });
}
