// SPDX-License-Identifier: MIT
import { nativeProduct } from "./profile.mjs";

import assert from "node:assert/strict";
import { once, EventEmitter } from "node:events";
import { mkdtemp, rm } from "node:fs/promises";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { Connection } from "@sessionbus/kit";
import { NativeOwners } from "./owners.mjs";
import { InteractiveEndpoint } from "./endpoint.mjs";
import { SessionbusForwarder } from "./forward.mjs";

function deferred() { let resolve; const promise = new Promise((yes) => { resolve = yes; }); return { promise, resolve }; }
const result = (data, status = 200) => ({ data, response: { status } });
const info = (id, title = "") => ({ id, title, directory: "/native/project" });
async function fixture(t, options = {}) {
  const directory = await mkdtemp(path.join(os.tmpdir(), "oc-own-"));
  const sockets = new Set(), wires = new Map(), calls = [], updates = [], failures = [];
  const hello = new EventEmitter(), events = new EventEmitter();
  const server = net.createServer((socket) => {
    sockets.add(socket); socket.once("close", () => sockets.delete(socket));
    let id;
    const connection = new Connection(socket, false, (request) => {
      void (async () => {
        if (request.method === "session.hello") {
          id = request.params.session_id; wires.set(id, connection); hello.emit(id, request.params);
          if (options.hello) await options.hello(connection, request);
          else await connection.result(request, {});
        } else {
          calls.push({ nativeID: id, ...request });
          if (options.call) await options.call(connection, request, id);
          else await connection.result(request, { sessions: [] });
        }
      })().catch((error) => connection.close(error));
    });
  });
  const socket = path.join(directory, "bus");
  server.listen(socket); await once(server, "listening");
  const api = { event: { on(type, handler) { events.on(type, handler); return () => events.off(type, handler); } }, client: { session: {
    async get(params, config) {
      assert.deepEqual(Object.keys(params), ["sessionID"]);
      assert.equal(config.throwOnError, true); assert.equal(config.redirect, "error");
      return options.get ? options.get(params, config) : result(info(params.sessionID));
    },
    async status(params, config) { return options.status ? options.status(params, config) : result({}); },
    async update(params, config) { updates.push(params); return options.update ? options.update(params, config) : result(info(params.sessionID, params.title)); },
    async promptAsync() { throw new Error("unexpected model input"); },
  } } };
  const owners = new NativeOwners(api, { socket, groups: ["group"], name: options.name || "" }, { report: (error) => failures.push(error), peer: options.peer });
  t.after(async () => {
    await owners.dispose();
    for (const stream of sockets) stream.destroy();
    await new Promise((resolve) => server.close(resolve));
    await rm(directory, { recursive: true, force: true });
  });
  const action = (id, signal = new AbortController().signal) => owners.action("list", {}, { sessionID: id, messageID: "msg_native", signal });
  return { owners, events, hello, wires, calls, updates, failures, action, sockets, directory };
}

test("native tool can establish unselected exact child; only first route elects name", { timeout: 5000 }, async (t) => {
  const f = await fixture(t, { name: "initial" });
  await f.action("ses_child");
  assert.equal(f.calls[0].nativeID, "ses_child");
  assert.deepEqual(f.updates, []);
  await f.owners.select("ses_selected");
  await f.owners.select("ses_later");
  assert.deepEqual(f.updates, [{ sessionID: "ses_selected", title: "initial" }]);
  assert.equal(f.wires.size, 3, "navigation retains established owners");
});

test("delayed native old-ID call remains old after route changes", { timeout: 5000 }, async (t) => {
  const entered = deferred(), release = deferred();
  const f = await fixture(t, { get: async ({ sessionID }) => {
    if (sessionID === "ses_old") { entered.resolve(); await release.promise; }
    return result(info(sessionID));
  } });
  const old = f.action("ses_old");
  await entered.promise;
  await f.owners.select("ses_new");
  await f.action("ses_new");
  release.resolve(); await old;
  assert.deepEqual(f.calls.map((call) => call.nativeID), ["ses_new", "ses_old"]);
});

test("deletion during held native GET forbids late owner resurrection", { timeout: 5000 }, async (t) => {
  const entered = deferred(), release = deferred();
  let signal;
  const f = await fixture(t, { get: async ({ sessionID }, options) => {
    signal = options.signal; entered.resolve(); await release.promise;
    return result(info(sessionID)); // Deliberately late native completion.
  } });
  const action = assert.rejects(f.action("ses_deleted"), /deleted/);
  await entered.promise;
  f.events.emit("session.deleted", { properties: { info: { id: "ses_deleted" } } });
  assert.equal(signal.aborted, true);
  release.resolve(); await action;
  await f.owners.dispose();
  assert.equal(f.wires.size, 0);
  assert.equal(f.calls.length, 0);
});

test("deletion during actual kit hello closes its owner before admission", { timeout: 5000 }, async (t) => {
  const entered = deferred();
  const f = await fixture(t, { hello: async () => { entered.resolve(); } });
  const action = assert.rejects(f.action("ses_held"), /deleted/);
  await entered.promise;
  const wire = f.wires.get("ses_held");
  const closed = once(wire.stream, "close");
  f.events.emit("session.deleted", { properties: { info: { id: "ses_held" } } });
  await action; await closed; await f.owners.dispose();
  assert.equal(f.calls.length, 0);
});

test("native TUI disposal joins pending native GET, even if abort response is delayed", { timeout: 5000 }, async (t) => {
  const entered = deferred(), release = deferred();
  let signal;
  const f = await fixture(t, { get: async ({ sessionID }, options) => {
    signal = options.signal; entered.resolve(); await release.promise; return result(info(sessionID));
  } });
  const action = assert.rejects(f.action("ses_held"), /disposed/);
  await entered.promise;
  let disposed = false;
  const closing = f.owners.dispose().then(() => { disposed = true; });
  assert.equal(signal.aborted, true); assert.equal(disposed, false);
  release.resolve(); await closing; await action;
  assert.equal(f.wires.size, 0);
});

test("initial native status query completes before first hello", { timeout: 5000 }, async (t) => {
  const entered = deferred(), release = deferred();
  const f = await fixture(t, { status: async () => {
    entered.resolve(); await release.promise; return result({ ses_busy: { type: "busy" } });
  } });
  const action = f.action("ses_busy");
  await entered.promise; assert.equal(f.wires.size, 0);
  release.resolve(); await action;
  assert.equal(f.wires.size, 1);
});

test("native blank title stays absent; same-ID title updates use rehello", { timeout: 5000 }, async (t) => {
  const firstHello = deferred();
  const f = await fixture(t, { hello: async (wire, request) => {
    firstHello.resolve(request.params); await wire.result(request, {});
  } });
  await f.action("ses_title");
  assert.equal(Object.hasOwn(await firstHello.promise, "name"), false);
  const wire = f.wires.get("ses_title");
  const renamed = once(f.hello, "ses_title");
  f.events.emit("session.updated", { properties: { info: info("ses_title", "native title") } });
  assert.equal((await renamed)[0].name, "native title");
  assert.equal(f.wires.get("ses_title"), wire);
  const blank = once(f.hello, "ses_title");
  f.events.emit("session.updated", { properties: { info: info("ses_title", "") } });
  assert.equal(Object.hasOwn((await blank)[0], "name"), false);
});

test("pending identity bound rejects seventeenth owner without native call", { timeout: 5000 }, async (t) => {
  const release = deferred(); let calls = 0;
  const f = await fixture(t, { get: async ({ sessionID }) => { calls++; await release.promise; return result(info(sessionID)); } });
  const pending = Array.from({ length: 16 }, (_, index) => f.action(`ses_${index}`));
  await assert.rejects(f.action("ses_overflow"), /owner limit/);
  release.resolve(); await Promise.all(pending);
  assert.equal(calls, 16);
});

test("invalid native title update retires old identity and joins without self-wait", { timeout: 5000 }, async (t) => {
  const f = await fixture(t);
  await f.action("ses_title");
  const wire = f.wires.get("ses_title"), closed = once(wire.stream, "close");
  f.events.emit("session.updated", { properties: { info: info("ses_title", "invalid\nname") } });
  await closed;
  await f.owners.dispose();
  assert.ok(f.failures.some((error) => /identity grammar/.test(error.message)));
});

test("retiring owner keeps capacity until held native delivery actually joins", { timeout: 10000 }, async (t) => {
  const entered = deferred(), release = deferred();
  let hold = false, signal;
  const f = await fixture(t, { status: async (_params, options) => {
    if (hold) { signal = options.signal; entered.resolve(); await release.promise; }
    return result({});
  } });
  for (let index = 0; index < 128; index++) await f.action(`ses_${index}`);
  hold = true;
  const wire = f.wires.get("ses_0");
  const delivery = wire.call("message.deliver", { message_id: "delivery", body: "hello", from: { session_id: "sender", product: "test", groups: [] } }).catch((error) => error);
  await entered.promise;
  f.events.emit("session.deleted", { properties: { info: { id: "ses_0" } } });
  assert.equal(signal.aborted, true);
  await assert.rejects(f.action("ses_overflow"), /owner limit/);
  release.resolve(); await delivery; await f.owners.dispose();
});

for (const boundary of ["route", "deletion"]) {
  test(`actual endpoint native metadata survives held GET and ${boundary}`, { timeout: 5000 }, async (t) => {
    const entered = deferred(), release = deferred();
    const f = await fixture(t, { get: async ({ sessionID }) => {
      if (sessionID === "ses_old") { entered.resolve(); await release.promise; }
      return result(info(sessionID));
    } });
    const socket = path.join(f.directory, "actions");
    const endpoint = new InteractiveEndpoint(socket, (action, args, context) => f.owners.action(action, args, context));
    await endpoint.ready();
    const client = new SessionbusForwarder(socket);
    t.after(async () => { await client.dispose(); await endpoint.dispose(); });
    const request = client.action("list", {}, { sessionID: "ses_old", messageID: "msg_actual_old" });
    const completion = boundary === "deletion" ? assert.rejects(request, /deleted/) : request;
    await entered.promise;
    await f.owners.select("ses_new");
    if (boundary === "deletion") f.events.emit("session.deleted", { properties: { info: { id: "ses_old" } } });
    release.resolve(); await completion;
    assert.deepEqual(f.calls.map((call) => call.nativeID), boundary === "deletion" ? [] : ["ses_old"]);
    await client.dispose(); await endpoint.dispose();
  });
}

test("deletion during initial native rename cannot rehello late title", { timeout: 5000 }, async (t) => {
  const entered = deferred(), release = deferred();
  const f = await fixture(t, { name: "first", update: async ({ sessionID, title }) => {
    entered.resolve(); await release.promise; return result(info(sessionID, title));
  } });
  const selecting = assert.rejects(f.owners.select("ses_initial"), /deleted/);
  await entered.promise;
  const wire = f.wires.get("ses_initial"), closed = once(wire.stream, "close");
  f.events.emit("session.deleted", { properties: { info: { id: "ses_initial" } } });
  release.resolve(); await selecting; await closed;
  await f.owners.select("ses_later");
  assert.deepEqual(f.updates, [{ sessionID: "ses_initial", title: "first" }]);
});

test("actual sole Caller preserves originating self_info with filtered rows", { timeout: 5000 }, async (t) => {
  const value = { sessions: [], self_info: { session_id: "ses_native@host", product: nativeProduct.product, groups: ["group"] } };
  const f = await fixture(t, { call: (wire, request) => wire.result(request, value) });
  assert.deepEqual(await f.action("ses_native"), value);
});
