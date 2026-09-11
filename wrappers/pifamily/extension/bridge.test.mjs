// SPDX-License-Identifier: MIT

import assert from "node:assert/strict";
import { once } from "node:events";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { mkdtemp, rm } from "node:fs/promises";
import test from "node:test";

import {
  BridgeBusyError,
  BridgeProtocolError,
  PrivateBridge,
  connectBridge,
} from "./bridge.mjs";

const lineStates = new WeakMap();

async function socketPair(t) {
  const directory = await mkdtemp(path.join(os.tmpdir(), "pifamily-bridge-"));
  const socketPath = path.join(directory, "bridge.sock");
  const server = net.createServer();
  server.listen(socketPath);
  await once(server, "listening");
  const accepted = once(server, "connection");
  const client = net.createConnection({ path: socketPath });
  await once(client, "connect");
  const [host] = await accepted;
  t.after(async () => {
    const waits = [];
    if (!host.destroyed) waits.push(once(host, "close"));
    if (!client.destroyed) waits.push(once(client, "close"));
    if (server.listening) waits.push(once(server, "close"));
    host.destroy();
    client.destroy();
    server.close();
    await Promise.allSettled(waits);
    await rm(directory, { recursive: true, force: true });
  });
  return [host, client];
}

async function bridgePair(t, hostHandler, nativeHandler, limits = undefined) {
  const [hostSocket, nativeSocket] = await socketPair(t);
  const host = new PrivateBridge(hostSocket, { role: "host", handler: hostHandler, limits });
  const native = new PrivateBridge(nativeSocket, { role: "native", handler: nativeHandler, limits });
  await Promise.all([host.ready(t.signal), native.ready(t.signal)]);
  t.after(async () => {
    await Promise.allSettled([host.close(), native.close()]);
  });
  return [host, native];
}

test("full-duplex call result and ownership accounting", async (t) => {
  let host;
  let native;
  [host, native] = await bridgePair(
    t,
    async ({ method, params, signal }) => {
      assert.equal(method, "native.outer");
      const inner = await host.call("host.inner", { value: "inner" }, { signal });
      return { value: `${params.value}:${inner.value}` };
    },
    async ({ method, params }) => {
      assert.equal(method, "host.inner");
      return { value: params.value === "inner" ? "native" : "wrong" };
    },
  );
  assert.deepEqual(await native.call("native.outer", { value: "outer" }, { signal: t.signal }), {
    value: "outer:native",
  });
  await waitForStats(t, host, zeroStats());
  await waitForStats(t, native, zeroStats());
});

test("cancelled call drains its response and preserves a healthy next call", async (t) => {
  let markStarted;
  const started = new Promise((resolve) => {
    markStarted = resolve;
  });
  let markFinished;
  const finished = new Promise((resolve) => {
    markFinished = resolve;
  });
  const [, native] = await bridgePair(t, async ({ method, signal }) => {
    if (method === "ping") return { ok: true };
    assert.equal(method, "hold");
    markStarted();
    try {
      await new Promise((resolve, reject) => {
        signal.addEventListener("abort", () => reject(signal.reason), { once: true });
      });
      return null;
    } finally {
      markFinished();
    }
  });
  const controller = new AbortController();
  const call = native.call("hold", {}, { signal: controller.signal });
  await started;
  controller.abort();
  await assert.rejects(call, (error) => error?.name === "AbortError");
  // The handler rejects on abort. Its finally path and response still have to
  // settle before the abandoned correlation disappears.
  assert.deepEqual(await native.call("ping", {}, { signal: t.signal }), { ok: true });
  await waitForStats(t, native, zeroStats());
  await finished;
});

test("pending-call capacity rejects before a second write", async (t) => {
  let markStarted;
  const started = new Promise((resolve) => {
    markStarted = resolve;
  });
  const [, native] = await bridgePair(
    t,
    async ({ signal }) => {
      markStarted();
      await new Promise((resolve, reject) => {
        signal.addEventListener("abort", () => reject(signal.reason), { once: true });
      });
    },
    undefined,
    { maxPendingCalls: 1 },
  );
  const controller = new AbortController();
  const first = native.call("hold", {}, { signal: controller.signal });
  await started;
  await assert.rejects(native.call("second", {}, { signal: t.signal }), BridgeBusyError);
  controller.abort();
  await assert.rejects(first, (error) => error?.name === "AbortError");
});

test("duplicate request and unknown response retire the connection", async (t) => {
  await t.test("duplicate request", async (t) => {
    const { bridge, raw } = await rawNative(t, async () => ({ ok: true }));
    raw.write('{"version":1,"type":"request","id":"n:1","method":"ping","params":{}}\n');
    const response = await nextLine(raw);
    assert.equal(JSON.parse(response).id, "n:1");
    raw.write('{"version":1,"type":"request","id":"n:1","method":"ping","params":{}}\n');
    await bridge.done;
    assert(bridge.failure instanceof BridgeProtocolError);
  });
  await t.test("unknown response", async (t) => {
    const { bridge, raw } = await rawNative(t);
    raw.write('{"version":1,"type":"response","id":"h:1","result":null}\n');
    await bridge.done;
    assert(bridge.failure instanceof BridgeProtocolError);
  });
});

test("oversized frames retire the connection", async (t) => {
  const { bridge, raw } = await rawNative(t, undefined, { maxFrameBytes: 256 });
  raw.write(
    `${JSON.stringify({
      version: 1,
      type: "request",
      id: "n:1",
      method: "ping",
      params: { text: "x".repeat(300) },
    })}\n`,
  );
  await bridge.done;
  assert(bridge.failure instanceof BridgeProtocolError);
});

test("rejected frame does not consume a request ID", async (t) => {
  const { bridge, raw } = await rawNative(t, undefined, { maxFrameBytes: 256 });
  await assert.rejects(
    bridge.call("oversized", { body: "x".repeat(300) }, { signal: t.signal }),
    BridgeProtocolError,
  );
  const call = bridge.call("ping", {}, { signal: t.signal });
  const request = JSON.parse(await nextLine(raw));
  assert.equal(request.id, "h:1");
  raw.write('{"version":1,"type":"response","id":"h:1","result":null}\n');
  assert.equal(await call, null);
});

test("hello is first even when the peer sent a request eagerly", async (t) => {
  const [hostSocket, raw] = await socketPair(t);
  raw.write('{"version":1,"type":"hello","role":"native"}\n');
  raw.write('{"version":1,"type":"request","id":"n:1","method":"ping","params":{}}\n');
  const bridge = new PrivateBridge(hostSocket, {
    role: "host",
    handler: async () => ({ ok: true }),
  });
  t.after(() => bridge.close().catch(() => {}));
  assert.equal(JSON.parse(await nextLine(raw)).type, "hello");
  const response = JSON.parse(await nextLine(raw));
  assert.equal(response.type, "response");
  assert.equal(response.id, "n:1");
});

test("held write plus retained response failure wakes its caller", async (t) => {
  const { bridge, raw } = await rawNative(t, undefined, {
    maxFrameBytes: 512,
    maxRetainedBytes: 512,
  });
  const originalWrite = bridge.socket.write.bind(bridge.socket);
  bridge.socket.write = (body, _callback) => originalWrite(body);
  const call = bridge.call("outbound", {}, { signal: t.signal });
  const request = JSON.parse(await nextLine(raw));
  assert.equal(request.id, "h:1");
  raw.write(
    `${JSON.stringify({ version: 1, type: "response", id: "h:1", result: "x".repeat(450) })}\n`,
  );
  await assert.rejects(call, BridgeBusyError);
  await bridge.done;
  assert.deepEqual(bridge.stats(), zeroStats());
});

test("accepted response wins cancellation before write callback", async (t) => {
  const { bridge, raw } = await rawNative(t);
  const originalWrite = bridge.socket.write.bind(bridge.socket);
  bridge.socket.write = (body, _callback) => originalWrite(body);
  const controller = new AbortController();
  const call = bridge.call("accepted", {}, { signal: controller.signal });
  const request = JSON.parse(await nextLine(raw));
  assert.equal(request.id, "h:1");
  raw.write('{"version":1,"type":"response","id":"h:1","result":{"value":"accepted"}}\n');
  await waitFor(t, () => {
    const stats = bridge.stats();
    return stats.pendingCalls === 0 && stats.pendingWrites === 1 && stats.retainedBytes > 0;
  });
  controller.abort();
  assert.deepEqual(await call, { value: "accepted" });
  assert.equal(bridge.failure, undefined);
  await bridge.close();
  assert.deepEqual(bridge.stats(), zeroStats());
});

test("connectBridge joins an actual socket close after constructor rejection", async (t) => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "pifamily-connect-"));
  const socketPath = path.join(directory, "bridge.sock");
  const server = net.createServer();
  server.listen(socketPath);
  await once(server, "listening");
  let observeClose;
  const accepted = new Promise((resolve) => {
    server.once("connection", (socket) => {
      observeClose = once(socket, "close");
      resolve();
    });
  });
  await assert.rejects(connectBridge(socketPath, { role: "invalid" }), TypeError);
  await accepted;
  await observeClose;
  server.close();
  await once(server, "close");
  await rm(directory, { recursive: true, force: true });
});

test("actual socket close settles buffered writes and joins handlers", async (t) => {
  let markStarted;
  const started = new Promise((resolve) => {
    markStarted = resolve;
  });
  const [hostSocket, nativeSocket] = await socketPair(t);
  const host = new PrivateBridge(hostSocket, {
    role: "host",
    handler: async ({ signal }) => {
      markStarted();
      await new Promise((resolve, reject) => {
        signal.addEventListener("abort", () => reject(signal.reason), { once: true });
      });
    },
  });
  const native = new PrivateBridge(nativeSocket, { role: "native" });
  await Promise.all([host.ready(t.signal), native.ready(t.signal)]);
  const call = native.call("hold", {}, { signal: t.signal });
  await started;
  nativeSocket.pause();
  const writes = Array.from({ length: 64 }, () =>
    host.call("large", { body: "x".repeat(900_000) }, { signal: t.signal }).catch((error) => error),
  );
  await Promise.resolve();
  const closing = host.close();
  await closing;
  nativeSocket.resume();
  await assert.rejects(call);
  await Promise.allSettled(writes);
  assert.deepEqual(host.stats(), zeroStats());
});

async function rawNative(t, handler = undefined, limits = undefined) {
  const [hostSocket, raw] = await socketPair(t);
  const bridge = new PrivateBridge(hostSocket, { role: "host", handler, limits });
  const hello = JSON.parse(await nextLine(raw));
  assert.deepEqual(hello, { version: 1, type: "hello", role: "host" });
  raw.write('{"version":1,"type":"hello","role":"native"}\n');
  await bridge.ready(t.signal);
  t.after(async () => {
    raw.destroy();
    await bridge.close().catch(() => {});
  });
  return { bridge, raw };
}

function nextLine(socket) {
  let state = lineStates.get(socket);
  if (!state) {
    state = { buffer: "", lines: [], waiters: [], error: undefined };
    lineStates.set(socket, state);
    socket.on("data", (chunk) => {
      state.buffer += chunk.toString("utf8");
      while (true) {
        const newline = state.buffer.indexOf("\n");
        if (newline < 0) break;
        const line = state.buffer.slice(0, newline);
        state.buffer = state.buffer.slice(newline + 1);
        const waiter = state.waiters.shift();
        if (waiter) waiter.resolve(line);
        else state.lines.push(line);
      }
    });
    socket.on("error", (error) => {
      state.error = error;
      for (const waiter of state.waiters.splice(0)) waiter.reject(error);
    });
  }
  if (state.lines.length > 0) return Promise.resolve(state.lines.shift());
  if (state.error) return Promise.reject(state.error);
  return new Promise((resolve, reject) => {
    state.waiters.push({ resolve, reject });
  });
}

function zeroStats() {
  return { pendingCalls: 0, activeCalls: 0, pendingWrites: 0, retainedBytes: 0 };
}

async function waitForStats(t, bridge, want) {
  await waitFor(t, () => {
    try {
      assert.deepEqual(bridge.stats(), want);
      return true;
    } catch {
      return false;
    }
  });
}

async function waitFor(t, predicate) {
  while (!predicate()) {
    if (t.signal?.aborted) throw t.signal.reason;
    await new Promise((resolve) => setImmediate(resolve));
  }
}
