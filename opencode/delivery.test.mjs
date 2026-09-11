// SPDX-License-Identifier: MIT

import assert from "node:assert/strict";
import test from "node:test";
import { NativeDelivery } from "./delivery.mjs";

const message = (n, body = "hello") => ({ message_id: `message_${n}`, body, from: { product: "test", session_id: "sender", groups: [] } });
function deferred() { let resolve; const promise = new Promise((yes) => { resolve = yes; }); return { promise, resolve }; }
function setup(t, overrides = {}) {
  const life = new AbortController(), submissions = [];
  let bytes = 0;
  const delivery = new NativeDelivery({ sessionID: "ses_native", signal: life.signal,
    status: async () => "idle", info: async () => ({ directory: "/native", agent: "native-agent", model: { providerID: "native-provider", id: "native-model", variant: "native-variant" } }),
    reserve: (size) => { bytes += size; return true; }, release: (size) => { bytes -= size; },
    submit: async (parameters, _signal, attempted) => { attempted(); submissions.push(parameters); },
    ...overrides,
  });
  t.after(async () => { life.abort(); await delivery.dispose(); assert.equal(bytes, 0); });
  return { delivery, life, submissions, bytes: () => bytes };
}

test("busy stage uses native idle event then one explicit native-session submission", async (t) => {
  let status = "busy";
  const f = setup(t, { status: async () => status });
  assert.equal((await f.delivery.enqueue(f.life.signal, message(1))).disposition, "queued_for_next_turn");
  assert.equal(f.submissions.length, 0); assert.ok(f.bytes() > 0);
  status = "idle"; await f.delivery.idle(); await f.delivery.idle();
  assert.equal(f.submissions.length, 1); assert.equal(f.bytes(), 0);
  const p = f.submissions[0];
  assert.equal(p.sessionID, "ses_native"); assert.equal(Object.hasOwn(p, "path"), false);
  assert.deepEqual(p.model, { providerID: "native-provider", modelID: "native-model" });
  assert.equal(p.agent, "native-agent"); assert.equal(p.variant, "native-variant");
  assert.match(p.parts[0].text, /message_1/);
});

test("attempted uncertain native handoff is never restored or retried", async (t) => {
  let attempts = 0;
  const f = setup(t, { submit: async (_p, _s, attempted) => { attempted(); attempts++; throw new Error("native response lost"); } });
  assert.deepEqual(await f.delivery.enqueue(f.life.signal, message(1)), { disposition: "rejected", reason: "native response lost" });
  await f.delivery.idle(); assert.equal(attempts, 1); assert.equal(f.bytes(), 0);
});

test("known pre-submit refusal preserves already receipted unsent input", async (t) => {
  let status = "busy", refuse = true, attempts = 0;
  const f = setup(t, { status: async () => status, submit: async (_p, _s, attempted) => {
    if (refuse) throw new Error("HTTP work bound"); attempted(); attempts++;
  } });
  assert.equal((await f.delivery.enqueue(f.life.signal, message(1))).disposition, "queued_for_next_turn");
  status = "idle"; await assert.rejects(f.delivery.idle(), /HTTP work bound/);
  assert.ok(f.bytes() > 0); assert.equal(attempts, 0);
  refuse = false; await f.delivery.idle(); assert.equal(attempts, 1); assert.equal(f.bytes(), 0);
});

test("per-owner FIFO count and byte limits reject without evicting prior input", async (t) => {
  const f = setup(t, { status: async () => "busy" });
  for (let index = 0; index < 64; index++) assert.equal((await f.delivery.enqueue(f.life.signal, message(index))).disposition, "queued_for_next_turn");
  assert.equal((await f.delivery.enqueue(f.life.signal, message(65))).disposition, "rejected");
  const second = setup(t, { status: async () => "busy" });
  assert.equal((await second.delivery.enqueue(second.life.signal, message(1, "x".repeat(1024 * 1024)))).disposition, "rejected");
  assert.equal(second.bytes(), 0);
});

test("owned cancellation joins an attempted native handoff without replay", async (t) => {
  const entered = deferred(), release = deferred(); let signal, attempts = 0;
  const f = setup(t, { submit: async (_p, cancel, attempted) => {
    signal = cancel; attempted(); attempts++; entered.resolve(); await release.promise; throw cancel.reason;
  } });
  const receipt = f.delivery.enqueue(f.life.signal, message(1));
  await entered.promise;
  f.life.abort(new Error("native deleted")); let closed = false;
  const disposing = f.delivery.dispose().then(() => { closed = true; });
  assert.equal(signal.aborted, true); assert.equal(closed, false);
  release.resolve(); await disposing; assert.equal((await receipt).disposition, "rejected");
  await f.delivery.idle(); assert.equal(attempts, 1);
});
