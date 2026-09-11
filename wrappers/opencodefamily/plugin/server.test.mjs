// SPDX-License-Identifier: MIT
import { nativeProduct } from "./profile.mjs";
import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile, stat } from "node:fs/promises";
import test from "node:test";
import { createServer } from "./server.mjs";
import { interactiveActivation, claimInteractive } from "./activation.mjs";
import { InteractiveEndpoint } from "./endpoint.mjs";

async function fixture(t) {
  const directory = await mkdtemp("/tmp/sb-server-");
  t.after(() => rm(directory, { recursive:true, force:true }));
  const binding = { directory, pid:process.ppid, socket:directory+"/bus.sock", name:"initial", groups:[] };
  return { directory, binding, environment: { [nativeProduct.launchEnv]: JSON.stringify(binding) } };
}
const native = { sessionID:"ses_actual", messageID:"msg_actual", abort:new AbortController().signal };

test("ordinary and inherited foreign-parent launches return no tool or owner", async (t) => {
  assert.deepEqual(await createServer({})(), {});
  const f = await fixture(t);
  f.binding.pid++;
  assert.deepEqual(await createServer({[nativeProduct.launchEnv]:JSON.stringify(f.binding)})(), {});
  assert.deepEqual(await stat(f.directory).then(async () => (await import("node:fs/promises")).readdir(f.directory)), []);
});

test("constructor returns before TUI readiness and native call metadata crosses resident endpoint", {timeout:5000}, async (t) => {
  const f = await fixture(t);
  const hooks = await createServer(f.environment)();
  t.after(() => hooks.dispose());
  assert.deepEqual(Object.keys(hooks.tool.sessionbus.args).sort(), ["action","arguments"]);
  const endpoint = new InteractiveEndpoint(f.directory+"/actions.sock", async (_action, _args, context) => ({id:context.sessionID, message:context.messageID}));
  t.after(() => endpoint.dispose());
  await endpoint.ready();
  await writeFile(f.directory+"/actions.ready", "", {flag:"wx", mode:0o600});
  const result = await hooks.tool.sessionbus.execute({action:"list",arguments:{}}, native);
  assert.deepEqual(JSON.parse(result), {id:"ses_actual",message:"msg_actual"});
});

test("cancelled startup calls release capacity; disposal joins missing-ready watcher", {timeout:5000}, async (t) => {
  const f = await fixture(t);
  const hooks = await createServer(f.environment)();
  for (let i=0;i<300;i++) {
    const cancel = new AbortController();
    const call = hooks.tool.sessionbus.execute({action:"list",arguments:{}}, {...native,abort:cancel.signal});
    const rejected = assert.rejects(call, /cancelled/);
    cancel.abort(new Error("cancelled"));
    await rejected;
  }
  const call = hooks.tool.sessionbus.execute({action:"list",arguments:{}}, native);
  const rejected = assert.rejects(call, /disposed/);
  await hooks.dispose();
  await rejected;
});

test("empty TUI claim admits exactly one overlapping owner and never transfers after failure", async (t) => {
  const f = await fixture(t);
  const launch = await interactiveActivation(f.environment);
  const claims = await Promise.allSettled([claimInteractive(launch), claimInteractive(launch)]);
  assert.equal(claims.filter((value) => value.status === "fulfilled").length, 1);
  assert.equal((await stat(f.directory+"/owner.claim")).size, 0);
  await assert.rejects(claimInteractive(launch), {code:"EEXIST"});
});
