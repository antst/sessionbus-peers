// SPDX-License-Identifier: MIT
import test from "node:test";
import assert from "node:assert/strict";
import { appendNative, describeNative } from "./native.mjs";

// Exercises the adapter boundary only. The installed Pi probe must bind the
// native same-stack append and guarded context access separately.
function fixture({ idle = true, append = true } = {}) {
  let leaf;
  let sends = 0;
  let valid = true;
  const read = () => { if (!valid) throw new Error("stale native context"); };
  const ctx = {
    cwd: "/native/project", isIdle: () => idle,
    sessionManager: {
      getSessionId: () => { read(); return "native-session"; },
      getLeafId: () => { read(); return leaf?.id ?? null; },
      getLeafEntry: () => { read(); return leaf; },
    },
  };
  const pi = {
    getSessionName: () => "native title",
    sendMessage(message, options) {
      sends++;
      assert.deepEqual(options, { triggerTurn: false });
      if (append) leaf = { type: "custom_message", id: `entry-${sends}`, parentId: leaf?.id ?? null, ...message };
    },
  };
  return { pi, ctx, sends: () => sends, invalidate: () => { valid = false; } };
}
const delivery = { session_id: "native-session", message_id: "message-1", body: "exact native envelope\n" };

test("native identity and exact leaf acknowledge an append without a turn", () => {
  const f = fixture();
  assert.deepEqual(describeNative(f.pi, f.ctx), { session_id: "native-session", name: "native title", cwd: "/native/project" });
  assert.deepEqual(appendNative(f.pi, f.ctx, delivery), { accepted: true, entry_id: "entry-1" });
  assert.deepEqual(appendNative(f.pi, f.ctx, { ...delivery, message_id: "message-2" }), { accepted: true, entry_id: "entry-2" });
  assert.equal(f.sends(), 2);
});

test("busy context retains wrapper ownership and makes no native submission", () => {
  const f = fixture({ idle: false });
  assert.deepEqual(appendNative(f.pi, f.ctx, delivery), { accepted: false, reason: "busy" });
  assert.equal(f.sends(), 0);
});

test("void API return without a matching native append is not a receipt", () => {
  const f = fixture({ append: false });
  assert.throws(() => appendNative(f.pi, f.ctx, delivery), /did not confirm/);
  assert.equal(f.sends(), 1);
});

test("wrong or stale session cannot receive the delivery", () => {
  const f = fixture();
  assert.throws(() => appendNative(f.pi, f.ctx, { ...delivery, session_id: "other" }), /different native session/);
  f.invalidate();
  assert.throws(() => appendNative(f.pi, f.ctx, delivery), /stale native context/);
  assert.equal(f.sends(), 0);
});

test("delivery identity and body bounds reject before native submission", () => {
  for (const change of [
    { message_id: "x".repeat(257) },
    { body: "x".repeat((1 << 20) + 1) },
  ]) {
    const f = fixture();
    assert.throws(() => appendNative(f.pi, f.ctx, { ...delivery, ...change }), /invalid Pi native delivery/);
    assert.equal(f.sends(), 0);
  }
  const f = fixture();
  assert.deepEqual(appendNative(f.pi, f.ctx, { ...delivery, message_id: "opaque message id" }),
    { accepted: true, entry_id: "entry-1" });
});

test("a substituted leaf cannot confirm this native call", () => {
  for (const change of [
    { type: "message" }, { content: "different" }, { details: { message_id: "other" } },
    { parentId: "foreign" }, { id: "" },
  ]) {
    const f = fixture();
    const original = f.ctx.sessionManager.getLeafEntry;
    f.ctx.sessionManager.getLeafEntry = () => ({ ...original(), ...change });
    assert.throws(() => appendNative(f.pi, f.ctx, delivery), /did not confirm/);
  }
});
