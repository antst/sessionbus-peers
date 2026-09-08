// SPDX-License-Identifier: MIT

import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import net from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { once } from "node:events";
import test from "node:test";

const ACTIONS = ["list", "send", "spawn", "describe", "run", "start", "wait", "status", "interrupt", "close", "forget"];

function fakeTool(definition) { return definition; }
fakeTool.schema = { enum: (values) => ({ values }), string: () => ({}), any: () => ({}), record: () => ({ default() { return this; } }) };

async function load(options = {}) {
  globalThis.__kit = { ACTIONS, connectPeer: options.connectPeer || (() => { throw new Error("unexpected peer"); }), validate() { return true; } };
  globalThis.__tool = fakeTool;
  let source = await readFile(new URL("./sessionbus.mjs", import.meta.url), "utf8");
  source = `const __testEnvironment = ${JSON.stringify(options.environment || {})};\n${source}`
    .replaceAll("process.env", "__testEnvironment")
    .replace('import kit from "@sessionbus/kit";', "const kit = globalThis.__kit;")
    .replace('import { tool } from "@opencode-ai/plugin";', "const tool = globalThis.__tool;");
  if (options.exposeEnvironment) source += "\nexport { __testEnvironment }; export const __testProcessEnvironment = typeof PROCESS_ENV === 'undefined' ? undefined : PROCESS_ENV;";
  return import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}#${Math.random()}`);
}

function peerFactory(records) {
  return (identity, deliver, environment) => {
    const peer = {
      identity, deliver, environment, rehellos: [], stopped: false,
      caller: { async action(action, argumentsValue) { records.actions.push({ identity, action, argumentsValue }); return { ok: true }; } },
      async rehello(signal, name, info) { assert.equal(signal, undefined); if (this.stopped) throw new Error("superseded"); this.rehellos.push({ name, info }); },
      shutdown() { this.stopped = true; },
    };
    records.peers.push(peer);
    return peer;
  };
}

function legacy(records, behavior = {}) {
  for (const name of ["statuses", "gets", "prompts"]) records[name] ||= [];
  return { session: {
    async status(request, options) {
      records.statuses.push({ request, options });
      return behavior.status ? behavior.status(request, options, records.statuses.length) : { response: { status: 200 }, data: {} };
    },
    async get(request, options) {
      records.gets.push({ request, options });
      return behavior.get ? behavior.get(request, options, records.gets.length) : { response: { status: 200 }, data: { id: request.path.id, agent: "build", model: { providerID: "opencode", id: "big-pickle", variant: "high" } } };
    },
    async promptAsync(request, options) {
      records.prompts.push({ request, options });
      return behavior.prompt ? behavior.prompt(request, options, records.prompts.length) : { response: { status: 204 } };
    },
  } };
}

function delivery(id, body = id) {
  return { message_id: id, from: { session_id: "from", name: "Sender", product: "codex", groups: [] }, body };
}

function deliveryID(value) {
  return `msg_${createHash("sha256").update(value).digest("hex").slice(0, 32)}`;
}

const context = { sessionID: "ses_exact", messageID: "msg_tool", abort: new AbortController().signal };

test("extracted package imports its exact kit dependency", async () => {
  const manifest = JSON.parse(await readFile(new URL("./package.json", import.meta.url), "utf8"));
  assert.equal(manifest.dependencies["@sessionbus/kit"], "0.1.0-pre.2");
  const module = await import("./sessionbus.mjs");
  assert.equal(typeof module.createPlugin, "function");
});

test("lane plugin is presence-inert and sends one stateless action", async () => {
  const module = await load({ environment: { SESSIONBUS_LANE_SOCKET: "/tmp/lane.sock", SESSIONBUS_GROUPS: "[]" } }), calls = [];
  const plugin = module.createPlugin({ tool: fakeTool, connectPeer() { throw new Error("lane created a peer"); }, privateAction: async (...args) => { calls.push(args); return { sessions: [] }; }, onExit() {} });
  const hooks = await plugin({ client: {}, directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "Lane title", directory: "/work" } } } });
  assert.deepEqual(hooks.tool.sessionbus.args.action.values, ACTIONS);
  assert.deepEqual(JSON.parse(await hooks.tool.sessionbus.execute({ action: "list", arguments: {} }, context)), { sessions: [] });
  assert.deepEqual(calls[0].slice(0, 3), ["/tmp/lane.sock", "list", {}]);
});

test("plugin stays discovery-safe without a Sessionbus connection environment", async () => {
  const module = await load(), records = { peers: [], actions: [], prompts: [] };
  const plugin = module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), onExit() {} });
  const hooks = await plugin({ client: {}, directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "Native", directory: "/work" } } } });
  assert.equal(records.peers.length, 0);
  await assert.rejects(hooks.tool.sessionbus.execute({ action: "list", arguments: {} }, context), /no live OpenCode peer/u);
});

test("module load captures and scrubs only Sessionbus environment", async () => {
  const environment = { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_LOCAL_KEY: "secret", SESSIONBUS_GROUPS: "[]", SESSIONBUS_FUTURE_VALUE: "future", OPENCODE_SERVER_USERNAME: "product", OPENCODE_SERVER_PASSWORD: "product-owned" };
  const module = await load({ environment, exposeEnvironment: true });
  assert.deepEqual(module.__testProcessEnvironment, { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_LOCAL_KEY: "secret", SESSIONBUS_GROUPS: "[]", SESSIONBUS_FUTURE_VALUE: "future" });
  assert.equal(Object.isFrozen(module.__testProcessEnvironment), true);
  assert.deepEqual(module.__testEnvironment, { OPENCODE_SERVER_USERNAME: "product", OPENCODE_SERVER_PASSWORD: "product-owned" });
  assert.equal(Object.hasOwn(module, "PROCESS_ENV"), false);
});

test("two same-process instances share the module environment snapshot", async () => {
  const records = { peers: [], actions: [], prompts: [] };
  const environment = { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_LOCAL_KEY: "secret", SESSIONBUS_GROUPS: "[]", OPENCODE_SERVER_USERNAME: "product", OPENCODE_SERVER_PASSWORD: "product-owned" };
  const module = await load({ environment, connectPeer: peerFactory(records) });
  const first = await module.default({ client: {}, directory: "/one" });
  const second = await module.default({ client: {}, directory: "/two" });
  await first.event({ event: { type: "session.created", properties: { info: { id: "ses_one", title: "One", directory: "/one" } } } });
  await second.event({ event: { type: "session.created", properties: { info: { id: "ses_two", title: "Two", directory: "/two" } } } });
  assert.deepEqual(records.peers.map((peer) => peer.identity.session_id), ["ses_one", "ses_two"]);
  assert.deepEqual(records.peers.map((peer) => peer.environment), [{ SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_LOCAL_KEY: "secret" }, { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_LOCAL_KEY: "secret" }]);
});

test("tool waits for the peer hello before its first action", async () => {
  let admit, actions = 0;
  const ready = new Promise((resolve) => { admit = resolve; });
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_GROUPS: "[]" } });
  const plugin = module.createPlugin({ tool: fakeTool, connectPeer: () => ({ ready, caller: { async action() { actions++; return { sessions: [] }; } }, shutdown() {} }), onExit() {} });
  const hooks = await plugin({ client: {}, directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "Exact", directory: "/work" } } } });
  const execution = hooks.tool.sessionbus.execute({ action: "list", arguments: {} }, context);
  await Promise.resolve();
  assert.equal(actions, 0);
  admit();
  assert.deepEqual(JSON.parse(await execution), { sessions: [] });
  assert.equal(actions, 1);
});

test("unknown update publishes a peer, retitles it, and deletion retires it", async () => {
  const environment = { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_LOCAL_KEY: "", SESSIONBUS_GROUPS: '["team"]' };
  const module = await load({ environment }), records = { peers: [], actions: [], prompts: [] };
  const plugin = module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), onExit() {} });
  const hooks = await plugin({ client: {}, directory: "/work" });
  assert.deepEqual(environment, { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_LOCAL_KEY: "", SESSIONBUS_GROUPS: '["team"]' });
  await hooks.event({ event: { type: "session.updated", properties: { sessionID: "ses_exact", info: { id: "ses_exact", title: "First title", directory: "/work" } } } });
  assert.deepEqual(records.peers[0].identity, { product: "opencode", session_id: "ses_exact", name: "First title", groups: ["team"], info: { cwd: "/work" } });
  assert.deepEqual(records.peers[0].environment, { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_LOCAL_KEY: "" });
  await hooks.event({ event: { type: "session.updated", properties: { sessionID: "ses_exact", info: { id: "ses_exact", title: "Second title", directory: "/next" } } } });
  assert.deepEqual(records.peers[0].rehellos, [{ name: "Second title", info: { cwd: "/next" } }]);
  await hooks.event({ event: { type: "session.deleted", properties: { info: { id: "ses_exact" } } } });
  assert.equal(records.peers[0].stopped, true);
});

test("created session publishes before its requested title update", async () => {
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_SESSION_NAME: "Named session", SESSIONBUS_GROUPS: "[]" } }), records = { peers: [], actions: [], prompts: [] }, updates = [];
  const plugin = module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), onExit() {} });
  const hooks = await plugin({ client: { session: { async update(request) { updates.push(request); return { response: { status: 200 }, data: { id: request.path.id, title: request.body.title } }; } } }, directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "New session", directory: "/work" } } } });
  assert.equal(updates.length, 1);
  assert.equal(records.peers[0].identity.name, "New session");
  assert.deepEqual(records.peers[0].rehellos, [{ name: "Named session", info: { cwd: "/work" } }]);
});

test("created session must confirm the exact retitled identity", async () => {
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_SESSION_NAME: "Named session", SESSIONBUS_GROUPS: "[]" } }), records = { peers: [], actions: [], prompts: [] };
  const plugin = module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), onExit() {} });
  const hooks = await plugin({ client: { session: { async update(request) { return { response: { status: 200 }, data: { id: `${request.path.id}-other`, title: request.body.title } }; } } }, directory: "/work" });
  await assert.rejects(hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "New session", directory: "/work" } } } }), /did not confirm/u);
  assert.equal(records.peers.length, 1);
  assert.equal(records.peers[0].identity.name, "New session");
  assert.deepEqual(records.peers[0].rehellos, []);
});

test("deleted session cannot be republished by a held title update", async () => {
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_SESSION_NAME: "Named session", SESSIONBUS_GROUPS: "[]" } }), records = { peers: [], actions: [], prompts: [] };
  let release;
  const held = new Promise((resolve) => { release = resolve; });
  const plugin = module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), onExit() {} });
  const hooks = await plugin({ client: { session: { async update(request) { await held; return { response: { status: 200 }, data: { id: request.path.id, title: request.body.title } }; } } }, directory: "/work" });
  const created = hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "New session", directory: "/work" } } } });
  assert.equal(records.peers.length, 1);
  await hooks.event({ event: { type: "session.deleted", properties: { info: { id: "ses_exact" } } } });
  assert.equal(records.peers[0].stopped, true);
  release();
  await created;
  assert.equal(records.peers.length, 1);
  assert.deepEqual(records.peers[0].rehellos, []);
  await assert.rejects(hooks.tool.sessionbus.execute({ action: "list", arguments: {} }, context), /no live OpenCode peer/u);
});

test("idle peer delivery uses the exact legacy session and waits for 204", async () => {
  let accept, enter;
  const accepted = new Promise((resolve) => { accept = resolve; });
  const entered = new Promise((resolve) => { enter = resolve; });
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_GROUPS: "[]" } }), records = { peers: [], actions: [] };
  const client = legacy(records, { prompt: () => { enter(); return accepted; } }), hooks = await module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), onExit() {} })({ client, directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "Exact", directory: "/work" } } } });
  assert.deepEqual(JSON.parse(await hooks.tool.sessionbus.execute({ action: "status", arguments: { turn_id: "t-1" } }, context)), { ok: true });
  let settled = false;
  const receipt = records.peers[0].deliver(context.abort, delivery("delivery", "hello"), records.peers[0].identity).then((value) => { settled = true; return value; });
  await entered;
  assert.equal(settled, false);
  accept({ response: { status: 204 } });
  assert.deepEqual(await receipt, { disposition: "queued_for_next_turn" });
  assert.deepEqual(records.statuses[0].request, { query: { directory: "/work" } });
  assert.deepEqual(records.gets[0].request, { path: { id: "ses_exact" }, query: { directory: "/work" } });
  assert.deepEqual(records.prompts[0].request.path, { id: "ses_exact" });
  assert.deepEqual(records.prompts[0].request.query, { directory: "/work" });
  assert.deepEqual(records.prompts[0].request.body, { messageID: deliveryID("delivery"), agent: "build", model: { providerID: "opencode", modelID: "big-pickle" }, variant: "high", parts: [{ type: "text", text: records.prompts[0].request.body.parts[0].text }] });
  assert.match(records.prompts[0].request.body.parts[0].text, /sessionbus-metadata/u);
});

test("busy peer delivery returns immediately and ignores later sender cancellation", async () => {
  let state = "busy", config = { id: "ses_exact", agent: "plan", model: { providerID: "before", id: "before", variant: "default" } };
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_GROUPS: "[]" } }), records = { peers: [], actions: [] };
  const client = legacy(records, { status: () => ({ response: { status: 200 }, data: { ses_exact: { type: state } } }), get: () => ({ response: { status: 200 }, data: config }) });
  const hooks = await module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), onExit() {} })({ client, directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "Exact", directory: "/work" } } } });
  const sender = new AbortController();
  assert.deepEqual(await records.peers[0].deliver(sender.signal, delivery("held"), records.peers[0].identity), { disposition: "queued_for_next_turn" });
  assert.equal(records.prompts.length, 0);
  sender.abort();
  config = { id: "ses_exact", agent: "build", model: { providerID: "opencode", id: "big-pickle", variant: "high" } };
  state = "idle";
  await hooks.event({ event: { type: "session.status", properties: { sessionID: "ses_exact", status: { type: "idle" } } } });
  assert.equal(records.prompts.length, 1);
  assert.equal(records.prompts[0].request.body.agent, "build");
  assert.deepEqual(records.prompts[0].request.body.model, { providerID: "opencode", modelID: "big-pickle" });
  assert.equal(records.prompts[0].request.body.variant, "high");
});

test("two busy deliveries preserve FIFO across two idle boundaries", async () => {
  let state = "busy";
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_GROUPS: "[]" } }), records = { peers: [], actions: [] };
  const client = legacy(records, {
    status: () => ({ response: { status: 200 }, data: { ses_exact: { type: state } } }),
    prompt: () => { state = "busy"; return { response: { status: 204 } }; },
  });
  const hooks = await module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), onExit() {} })({ client, directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "Exact", directory: "/work" } } } });
  assert.deepEqual(await records.peers[0].deliver(context.abort, delivery("first"), records.peers[0].identity), { disposition: "queued_for_next_turn" });
  assert.deepEqual(await records.peers[0].deliver(context.abort, delivery("second"), records.peers[0].identity), { disposition: "queued_for_next_turn" });
  state = "idle";
  await hooks.event({ event: { type: "session.status", properties: { sessionID: "ses_exact", status: { type: "idle" } } } });
  assert.deepEqual(records.prompts.map((entry) => entry.request.body.messageID), [deliveryID("first")]);
  state = "idle";
  await hooks.event({ event: { type: "session.status", properties: { sessionID: "ses_exact", status: { type: "idle" } } } });
  assert.deepEqual(records.prompts.map((entry) => entry.request.body.messageID), [deliveryID("first"), deliveryID("second")]);
});

test("idle crossing a held busy observation drains the queued delivery", async () => {
  let release;
  const held = new Promise((resolve) => { release = resolve; });
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_GROUPS: "[]" } }), records = { peers: [], actions: [] };
  const client = legacy(records, { status: (_request, _options, count) => count === 1 ? held : { response: { status: 200 }, data: {} } });
  const hooks = await module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), onExit() {} })({ client, directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "Exact", directory: "/work" } } } });
  const receipt = records.peers[0].deliver(context.abort, delivery("crossed"), records.peers[0].identity);
  await Promise.resolve();
  const idle = hooks.event({ event: { type: "session.status", properties: { sessionID: "ses_exact", status: { type: "idle" } } } });
  release({ response: { status: 200 }, data: { ses_exact: { type: "busy" } } });
  assert.deepEqual(await receipt, { disposition: "queued_for_next_turn" });
  await idle;
  assert.equal(records.statuses.length, 2);
  assert.equal(records.prompts.length, 1);
});

test("late native rejection is reported once and the next item drains later", async () => {
  let state = "busy";
  const reported = [];
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_GROUPS: "[]" } }), records = { peers: [], actions: [] };
  const client = legacy(records, { status: () => ({ response: { status: 200 }, data: { ses_exact: { type: state } } }), prompt: (_request, _options, count) => count === 1 ? { response: { status: 400 }, error: { message: "bad" } } : { response: { status: 204 } } });
  const hooks = await module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), reportError: (error) => { reported.push(error.message); throw new Error("reporter failed"); }, onExit() {} })({ client, directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "Exact", directory: "/work" } } } });
  assert.deepEqual(await records.peers[0].deliver(context.abort, delivery("rejected"), records.peers[0].identity), { disposition: "queued_for_next_turn" });
  assert.deepEqual(await records.peers[0].deliver(context.abort, delivery("next"), records.peers[0].identity), { disposition: "queued_for_next_turn" });
  state = "idle";
  await hooks.event({ event: { type: "session.status", properties: { sessionID: "ses_exact", status: { type: "idle" } } } });
  assert.deepEqual(reported, ["OpenCode legacy prompt lacked exact native acceptance"]);
  assert.equal(records.prompts.length, 1);
  await hooks.event({ event: { type: "session.status", properties: { sessionID: "ses_exact", status: { type: "idle" } } } });
  assert.equal(records.prompts.length, 2);
  assert.equal(reported.length, 1);
});

test("idle rejection and malformed status take truthful direct or reported errors", async () => {
  let malformed;
  const reported = [];
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_GROUPS: "[]" } }), records = { peers: [], actions: [] };
  const client = legacy(records, { status: () => ({ response: { status: 200 }, data: malformed || {} }), prompt: () => ({ response: { status: 400 }, error: { message: "bad" } }) });
  const hooks = await module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), reportError: (error) => reported.push(error.message), onExit() {} })({ client, directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "Exact", directory: "/work" } } } });
  await assert.rejects(records.peers[0].deliver(context.abort, delivery("rejected"), records.peers[0].identity), /lacked exact native acceptance/u);
  malformed = [];
  await assert.rejects(records.peers[0].deliver(context.abort, delivery("malformed-map"), records.peers[0].identity), /legacy status is malformed/u);
  malformed = { ses_exact: {} };
  await assert.rejects(records.peers[0].deliver(context.abort, delivery("malformed"), records.peers[0].identity), /legacy status is malformed/u);
  malformed = { ses_exact: { type: "unknown" } };
  await assert.rejects(records.peers[0].deliver(context.abort, delivery("unknown-status"), records.peers[0].identity), /legacy status is malformed/u);
  malformed = { ses_exact: { type: "busy" } };
  assert.deepEqual(await records.peers[0].deliver(context.abort, delivery("reported"), records.peers[0].identity), { disposition: "queued_for_next_turn" });
  malformed = { ses_exact: {} };
  await hooks.event({ event: { type: "session.status", properties: { sessionID: "ses_exact", status: { type: "idle" } } } });
  assert.deepEqual(reported, ["OpenCode legacy status is malformed"]);
  assert.equal(records.prompts.length, 1);
});

test("unknown submission outcome and pre-submit cancellation are never retried", async () => {
  let state = "busy", fail = true, cancelMode = false;
  const reported = [];
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_GROUPS: "[]" } }), records = { peers: [], actions: [] };
  const client = legacy(records, { status: (_request, options) => {
    if (cancelMode) return new Promise((_resolve, reject) => options.signal.addEventListener("abort", () => reject(options.signal.reason), { once: true }));
    return { response: { status: 200 }, data: { ses_exact: { type: state } } };
  }, prompt: () => { if (fail) throw new Error("socket lost"); return { response: { status: 204 } }; } });
  const hooks = await module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), reportError: (error) => reported.push(error.message), onExit() {} })({ client, directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "Exact", directory: "/work" } } } });
  assert.deepEqual(await records.peers[0].deliver(context.abort, delivery("unknown"), records.peers[0].identity), { disposition: "queued_for_next_turn" });
  state = "idle";
  await hooks.event({ event: { type: "session.status", properties: { sessionID: "ses_exact", status: { type: "idle" } } } });
  assert.deepEqual(reported, ["OpenCode native delivery outcome is unknown"]);
  fail = false;
  await hooks.event({ event: { type: "session.status", properties: { sessionID: "ses_exact", status: { type: "idle" } } } });
  assert.equal(records.prompts.length, 1);
  const cancelled = new AbortController();
  cancelMode = true;
  const pending = records.peers[0].deliver(cancelled.signal, delivery("cancelled"), records.peers[0].identity);
  await Promise.resolve();
  cancelled.abort(new Error("cancelled"));
  await assert.rejects(pending, /cancelled/u);
  cancelMode = false;
  await hooks.event({ event: { type: "session.status", properties: { sessionID: "ses_exact", status: { type: "idle" } } } });
  assert.equal(records.prompts.length, 1);
});

test("session deletion drops queued delivery and retires the peer", async () => {
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_GROUPS: "[]" } }), records = { peers: [], actions: [] };
  const client = legacy(records, { status: () => ({ response: { status: 200 }, data: { ses_exact: { type: "busy" } } }) });
  const hooks = await module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), onExit() {} })({ client, directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "Exact", directory: "/work" } } } });
  assert.deepEqual(await records.peers[0].deliver(context.abort, delivery("drop"), records.peers[0].identity), { disposition: "queued_for_next_turn" });
  await hooks.event({ event: { type: "session.deleted", properties: { info: { id: "ses_exact" } } } });
  assert.deepEqual(await records.peers[0].deliver(context.abort, delivery("retired"), records.peers[0].identity), { disposition: "rejected", reason: "closing" });
  await hooks.event({ event: { type: "session.status", properties: { sessionID: "ses_exact", status: { type: "idle" } } } });
  assert.equal(records.prompts.length, 0);
  assert.equal(records.peers[0].stopped, true);
});

test("peer delivery matches the shared native-message fixture", async () => {
  const module = await load({ environment: { SESSIONBUS_SOCKET: "/tmp/bus.sock", SESSIONBUS_GROUPS: "[]" } }), records = { peers: [], actions: [] };
  const hooks = await module.createPlugin({ tool: fakeTool, connectPeer: peerFactory(records), onExit() {} })({ client: legacy(records), directory: "/work" });
  await hooks.event({ event: { type: "session.created", properties: { info: { id: "ses_exact", title: "Exact", directory: "/work" } } } });
  const fixture = JSON.parse(await readFile(new URL("../wrappers/host/testdata/native-message-envelope.json", import.meta.url), "utf8"));
  await records.peers[0].deliver(context.abort, fixture.message, records.peers[0].identity);
  assert.equal(records.prompts[0].request.body.parts[0].text, fixture.rendered);
});

test("lane hop accepts one bounded frame and rejects trailing or empty replies", async () => {
  const directory = await mkdtemp(join(tmpdir(), "sessionbus-opencode-"));
  let index = 0;
  const socketPath = join(directory, "lane.sock");
  const module = await load({ environment: { SESSIONBUS_LANE_SOCKET: socketPath, SESSIONBUS_GROUPS: "[]" } });
  const responses = ['{"result":{"sessions":[]}}\n', '{"result":{}}\n{"result":{}}\n', '{"result":{},"error":{"code":-32603,"message":"bad"}}\n', null];
  const server = net.createServer((socket) => socket.once("data", () => {
    const body = responses[index++];
    if (body === null) return socket.destroy();
    socket.write(body.slice(0, 8));
    socket.end(body.slice(8));
  }));
  server.listen(socketPath);
  await once(server, "listening");
  try {
    const hooks = await module.createPlugin({ tool: fakeTool, onExit() {} })({ client: {}, directory: "/work" });
    assert.deepEqual(JSON.parse(await hooks.tool.sessionbus.execute({ action: "list", arguments: {} }, context)), { sessions: [] });
    await assert.rejects(hooks.tool.sessionbus.execute({ action: "list", arguments: {} }, context), /trailing frame/u);
    await assert.rejects(hooks.tool.sessionbus.execute({ action: "list", arguments: {} }, context), /exactly one result or error/u);
    await assert.rejects(hooks.tool.sessionbus.execute({ action: "list", arguments: {} }, context), /empty response/u);
  } finally {
    server.close();
    await once(server, "close");
    await rm(directory, { recursive: true, force: true });
  }
});

test("shell ingress requires and exports the exact native session", async () => {
  const module = await load({ environment: { SESSIONBUS_LANE_SOCKET: "/tmp/lane", SESSIONBUS_GROUPS: "[]" } });
  const hooks = await module.createPlugin({ tool: fakeTool, onExit() {} })({ client: {}, directory: "/work" });
  const output = { env: {} };
  await hooks["shell.env"]({ sessionID: "ses_exact" }, output);
  assert.equal(output.env.SESSIONBUS_SESSION_ID, "ses_exact");
  await assert.rejects(hooks["shell.env"]({}, { env: {} }), /exact OpenCode session/u);
});
