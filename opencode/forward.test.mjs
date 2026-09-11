// SPDX-License-Identifier: MIT
import assert from "node:assert/strict";
import net from "node:net";
import { mkdtemp, rm } from "node:fs/promises";
import { once } from "node:events";
import test from "node:test";
import { SessionbusForwarder, bridgeLimits } from "./forward.mjs";

async function fixture(t, handler) {
  const directory = await mkdtemp("/tmp/sb-oc-");
  const socket = directory + "/mcp.sock";
  let connections = 0;
  const clients = new Set();
  const server = net.createServer((client) => {
    connections++;
    clients.add(client);
    client.on("error", () => {});
    client.on("close", () => clients.delete(client));
    let text = "";
    const reply = (id, result) => client.write(JSON.stringify({ jsonrpc: "2.0", id, result }) + "\n");
    client.on("data", (chunk) => {
      text += chunk;
      while (text.includes("\n")) {
        const index = text.indexOf("\n");
        const value = JSON.parse(text.slice(0, index));
        text = text.slice(index + 1);
        if (value.method === "initialize") reply(value.id, { protocolVersion: "2024-11-05", capabilities: { tools: {} } });
        else if (value.method === "tools/list") reply(value.id, { tools: [{ name: "sessionbus" }] });
        else if (value.method === "notifications/initialized") continue;
        else handler(value, client, reply);
      }
    });
  });
  server.listen(socket);
  await once(server, "listening");
  const forward = new SessionbusForwarder(socket);
  t.after(async () => {
    await forward.dispose();
    for (const client of clients) client.destroy();
    await new Promise((resolve) => server.close(resolve));
    await rm(directory, { recursive: true });
  });
  await forward.ready();
  return { forward, connections: () => connections };
}

const identity = { sessionID: "ses_actual", messageID: "msg_actual" };

test("concurrent native contexts stay per call; cancellation needs no reply", async (t) => {
  let enter;
  const entered = new Promise((resolve) => { enter = resolve; });
  let cancelSeen;
  const cancelled = new Promise((resolve) => { cancelSeen = resolve; });
  const { forward } = await fixture(t, (value, _client, reply) => {
    if (value.method === "notifications/cancelled") { cancelSeen(value.params.requestId); return; }
    if (value.params.arguments.action === "wait") { enter(value.id); return; }
    reply(value.id, { content: [{ type: "text", text: JSON.stringify(value.params._meta) }] });
  });
  const abort = new AbortController();
  const pending = forward.action("wait", {}, { ...identity, abort: abort.signal });
  const rejection = assert.rejects(pending, /cancelled/);
  const id = await entered;
  abort.abort(new Error("cancelled"));
  await rejection;
  assert.equal(await cancelled, id);
  const values = await Promise.all(["one", "two"].map((suffix) => forward.action("list", {}, { sessionID: "ses_" + suffix, messageID: "msg_" + suffix })));
  assert.deepEqual(values, ["one", "two"].map((suffix) => ({ "sessionbus.opencode": { session_id: "ses_" + suffix, message_id: "msg_" + suffix } })));
});

for (const [name, emit] of [
  ["null error", (id) => JSON.stringify({ jsonrpc: "2.0", id, error: null }) + "\n"],
  ["future id", (id) => JSON.stringify({ jsonrpc: "2.0", id: id + 1, result: {} }) + "\n"],
  ["invalid UTF8", () => Buffer.from([255, 10])],
  ["oversized frame", () => " ".repeat(bridgeLimits.response + 1)],
]) {
  test(name + " retires connection without reconnect", async (t) => {
    const { forward, connections } = await fixture(t, (value, client) => client.write(emit(value.id)));
    await assert.rejects(forward.action("list", {}, identity));
    await assert.rejects(forward.action("list", {}, identity));
    await forward.dispose();
    assert.equal(connections(), 1);
  });
}

test("prewrite oversized call stays healthy and sends no request", async (t) => {
  let calls = 0;
  const { forward } = await fixture(t, (value, _client, reply) => {
    calls++;
    reply(value.id, { content: [{ type: "text", text: "{}" }] });
  });
  await assert.rejects(forward.action("send", { message: "x".repeat(bridgeLimits.input) }, identity), /2 MiB/);
  assert.deepEqual(await forward.action("list", {}, identity), {});
  assert.equal(calls, 1);
});

test("disposal joins socket while native action has no response", async (t) => {
  let enter;
  const entered = new Promise((resolve) => { enter = resolve; });
  const { forward } = await fixture(t, () => enter());
  const action = forward.action("list", {}, identity);
  const rejected = assert.rejects(action, /disposed/);
  await entered;
  await forward.dispose();
  await rejected;
});

test("native work capacity rejects admission and disposal settles all admitted calls", async (t) => {
  let enter;
  const entered = new Promise((resolve) => { enter = resolve; });
  let count = 0;
  const { forward } = await fixture(t, () => { if (++count === bridgeLimits.work) enter(); });
  const pending = Array.from({ length: bridgeLimits.work }, () => forward.action("wait", {}, identity));
  const all = Promise.allSettled(pending);
  await entered;
  await assert.rejects(forward.action("list", {}, identity), /work limit/);
  await forward.dispose();
  assert.equal((await all).filter((result) => result.status === "rejected").length, bridgeLimits.work);
});

test("actual close settles writes when runtime omits callbacks; destroy alone does not", async () => {
  const { EventEmitter } = await import("node:events");
  const { setImmediate: nextTurn } = await import("node:timers/promises");
  class HeldSocket extends EventEmitter {
    destroyed = false;
    held = [];
    write(frame, callback) {
      const value = JSON.parse(frame.toString());
      if (value.method === "tools/call") { this.held.push(callback); return false; }
      queueMicrotask(() => {
        callback();
        const result = value.method === "initialize"
          ? { protocolVersion: "2024-11-05", capabilities: { tools: {} } }
          : value.method === "tools/list" ? { tools: [{ name: "sessionbus" }] } : undefined;
        if (result) this.emit("data", Buffer.from(JSON.stringify({ jsonrpc: "2.0", id: value.id, result }) + "\n"));
      });
      return true;
    }
    destroy() { this.destroyed = true; return this; }
  }
  const socket = new HeldSocket();
  const connect = net.createConnection;
  let forward;
  net.createConnection = () => socket;
  try { forward = new SessionbusForwarder("/fixture/held"); }
  finally { net.createConnection = connect; }
  socket.emit("connect");
  await forward.ready();
  const rejected = assert.rejects(forward.action("list", {}, identity), /disposed/);
  await nextTurn();
  assert.equal(socket.held.length, 1);
  let settled = false;
  const disposing = forward.dispose().then(() => { settled = true; });
  try {
    await nextTurn();
    assert.equal(socket.destroyed, true);
    assert.equal(settled, false, "destroy invocation is not actual close");
    socket.emit("close");
    await nextTurn();
    assert.equal(settled, true, "actual close must settle write ownership without runtime callbacks");
    for (const callback of socket.held) { callback(new Error("late callback")); callback(); }
    await forward.dispose();
    await rejected;
  } finally {
    socket.emit("close");
    for (const callback of socket.held) callback();
    await disposing;
    await rejected;
  }
});
