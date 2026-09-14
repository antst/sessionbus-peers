// SPDX-License-Identifier: MIT
// Development fixture: native-shaped caller against the actual Go MCP engine.
import assert from "node:assert/strict";
import { once } from "node:events";
import { SessionbusForwarder } from "./forward.mjs";

const mode = process.argv[3];
const lifetime = new AbortController();
const forward = new SessionbusForwarder(process.argv[2], lifetime.signal);
const identity = { sessionID: "ses_native", messageID: "msg_native" };
try {
  await forward.ready();
  const result = await forward.action("list", {}, identity);
  assert.equal(result.escaped, "<\n\\\"".repeat(65536));
  const cancel = new AbortController();
  const action = forward.action("wait", { session_id: "lane" }, { ...identity, abort: cancel.signal });
  const settled = action.then(() => { throw new Error("held action unexpectedly succeeded"); }, (error) => error);
  if (mode === "cancel") {
    await once(process.stdin, "data"); // parent observed actual ActionWithMeta entry
    cancel.abort(new Error("native abort"));
    assert.match((await settled).message, /native abort/);
    const next = await forward.action("list", {}, identity);
    assert.equal(next.escaped, result.escaped);
  } else {
    assert.match((await settled).message, /ended|closed|reset/i);
  }
} finally {
  await forward.dispose();
  process.stdin.destroy();
}
console.log("PASS " + mode);
