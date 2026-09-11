// SPDX-License-Identifier: MIT
import assert from "node:assert/strict";
import { mkdtemp, stat, rm } from "node:fs/promises";
import test from "node:test";
import net from "node:net";
import { once } from "node:events";
import { InteractiveEndpoint } from "./endpoint.mjs";
import { SessionbusForwarder } from "./forward.mjs";

async function fixture(t, action) {
  const directory = await mkdtemp("/tmp/sb-ep-");
  const path = directory + "/actions.sock";
  const endpoint = new InteractiveEndpoint(path, action);
  const clients = [];
  t.after(async () => {
    await Promise.all(clients.map((client) => client.dispose()));
    await endpoint.dispose();
    await rm(directory, { recursive: true, force: true });
  });
  await endpoint.ready();
  assert.equal((await stat(path)).mode & 0o777, 0o600);
  return { endpoint, connect() { const client = new SessionbusForwarder(path); clients.push(client); return client; } };
}

const identity = { sessionID: "ses_root", messageID: "msg_actual" };

test("actual resident endpoint routes simultaneous native identities and preserves escaped results", async (t) => {
  const f = await fixture(t, async (action, args, native) => ({ action, args, id: native.sessionID, message: native.messageID, escaped: "<\n\\\"".repeat(65536) }));
  const client = f.connect();
  const values = await Promise.all(["old", "new"].map((suffix) => client.action("list", {}, { sessionID: "ses_"+suffix, messageID: "msg_"+suffix })));
  assert.deepEqual(values.map((value) => [value.id, value.message]), [["ses_old", "msg_old"], ["ses_new", "msg_new"]]);
  assert.equal(values[0].escaped, "<\n\\\"".repeat(65536));
});

for (const mode of ["cancel", "eof", "dispose"]) {
  test("actual endpoint " + mode + " cancels and joins held action", async (t) => {
    let enter, finish;
    const entered = new Promise((resolve) => { enter = resolve; });
    const finished = new Promise((resolve) => { finish = resolve; });
    const f = await fixture(t, async (action, _args, native) => {
      if (action !== "wait") return { healthy: true };
      enter();
      await new Promise((resolve) => native.signal.addEventListener("abort", resolve, { once: true }));
      finish();
      throw native.signal.reason;
    });
    const client = f.connect();
    const abort = new AbortController();
    const action = client.action("wait", {}, { ...identity, abort: abort.signal });
    const rejected = assert.rejects(action);
    await entered;
    if (mode === "cancel") abort.abort();
    else if (mode === "eof") await client.dispose();
    else await f.endpoint.dispose();
    await rejected;
    await finished;
    if (mode === "cancel") assert.deepEqual(await client.action("list", {}, identity), { healthy: true });
  });
}

test("ninth resident connection rejected without retiring existing eight", async (t) => {
  const f = await fixture(t, async () => ({}));
  const clients = Array.from({ length: 8 }, () => f.connect());
  await Promise.all(clients.map((client) => client.ready()));
  await assert.rejects(f.connect().ready());
  assert.deepEqual(await clients[0].action("list", {}, identity), {});
});

test("global work budget spans resident connections", async (t) => {
  let enter;
  const entered = new Promise((resolve) => { enter = resolve; });
  let count = 0, cancelled = 0;
  const f = await fixture(t, async (_action, _args, native) => {
    if (++count === 256) enter();
    await new Promise((resolve) => native.signal.addEventListener("abort", resolve, { once: true }));
    cancelled++;
    throw native.signal.reason;
  });
  const a = f.connect(), b = f.connect(), excess = f.connect();
  await Promise.all([a.ready(), b.ready(), excess.ready()]);
  const pending = [a,b].flatMap((client) => Array.from({length:128}, () => client.action("wait", {}, identity)));
  const settled = Promise.allSettled(pending);
  await entered;
  await assert.rejects(excess.action("list", {}, identity));
  assert.equal(count, 256);
  assert.equal(cancelled, 0);
  await f.endpoint.dispose();
  assert.equal(cancelled, 256);
  assert.equal((await settled).filter((result) => result.status === "rejected").length, 256);
});

test('actual endpoint close settles writes when runtime omits callbacks', {timeout:3000}, async () => {
  const directory = await mkdtemp('/tmp/oc-review-close-');
  const original = net.createServer;
  const held = [];
  let serverSocket, signalWrite;
  const written = new Promise(resolve => { signalWrite=resolve; });
  net.createServer = (listener) => original(socket => {
    serverSocket=socket;
    const write=socket.write;
    socket.write=function(frame, callback) {
      return write.call(this, frame, (...args) => { held.push(() => callback(...args)); signalWrite(); });
    };
    listener(socket);
  });
  let endpoint;
  try { endpoint=new InteractiveEndpoint(directory+'/actions.sock', async()=>({})); }
  finally { net.createServer=original; }
  let client, timer, closing;
  try {
    await endpoint.ready();
    client=net.createConnection(directory+'/actions.sock');
    client.on('error',()=>{}); client.resume();
    await once(client,'connect');
    client.write(JSON.stringify({jsonrpc:'2.0',id:1,method:'ping',params:{}})+'\n');
    await written;
    const closed=once(serverSocket,'close');
    closing=endpoint.dispose();
    await closed;
    assert.equal(serverSocket.destroyed,true);
    const result=await Promise.race([closing.then(()=>true),new Promise(resolve=>{timer=setTimeout(()=>resolve(false),500);})]);
    assert.equal(result,true,'endpoint disposal remained pending after actual socket close with callback omitted');
    for(const callback of held) callback();
    await endpoint.dispose();
  } finally {
    clearTimeout(timer);
    for(const callback of held.splice(0)) callback();
    client?.destroy();
    await closing;
    await endpoint.dispose();
    await rm(directory,{recursive:true,force:true});
  }
});
