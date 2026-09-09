// SPDX-License-Identifier: MIT
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:net';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { once } from 'node:events';
import { Connection } from '@sessionbus/kit';
import { Owner } from './owner.mjs';

// Controlled local wire endpoints; no daemon, product, sleeps or polling.
export async function wire(t, options = {}) {
  const dir=mkdtempSync(join(tmpdir(),'claude-wire-')), path=join(dir,'s');
  const requests=[], readers=[]; let count=0; const connections=[];
  const server=createServer(stream => {
    count++; const c=new Connection(stream,false,r=>{
      const item={c,r}; if(readers.length) readers.shift()(item); else requests.push(item);
    }); connections.push(c);
  });
  server.listen(path); await once(server,'listening');
  const owner=new Owner({env:{SESSIONBUS_SOCKET:path},...options});
  t.after(async()=>{owner.end(); connections.forEach(c=>c.close()); await new Promise(r=>server.close(r)); rmSync(dir,{recursive:true,force:true});});
  return {owner, next:()=>requests.length?Promise.resolve(requests.shift()):new Promise(r=>readers.push(r)), count:()=>count};
}
export const prompt=(id='native-a',title)=>({hook_event_name:'UserPromptSubmit',session_id:id,...(title===undefined?{}:{session_title:title})});
export async function publish(w,event=prompt()) {
  const ready=w.owner.report(event), {c,r}=await w.next(); assert.equal(r.method,'session.hello');
  await c.result(r,{}); await ready; return {c,r};
}

test('usable Stop can publish absent name; next prompt names; empty title preserves it',async t=>{
  const w=await wire(t); assert.equal(w.count(),0); assert.throws(()=>w.owner.action('list',{}),/not_connected/);
  const first=await publish(w,{hook_event_name:'Stop',session_id:'native-a',session_title:'${session_title}'});
  assert.equal(Object.hasOwn(first.r.params,'name'),false);
  const named=await publish(w,prompt('native-a','Native title')); assert.equal(named.r.params.name,'Native title');
  await w.owner.report({hook_event_name:'Stop',session_id:'native-a',session_title:''});
  assert.equal(w.owner.admitted.name,'Native title'); assert.equal(w.count(),1);
});
test('nonmatching End cannot withdraw; matching End allows clear and a new connection',async t=>{
  const w=await wire(t); await publish(w);
  await w.owner.report({hook_event_name:'SessionEnd',session_id:'other'}); assert.ok(w.owner.admitted);
  await w.owner.report({hook_event_name:'SessionEnd',session_id:'native-a'}); assert.equal(w.owner.admitted,null);
  await publish(w,prompt('native-b')); assert.equal(w.count(),2); assert.equal(w.owner.admitted.session_id,'native-b');
});
test('obsolete hello acknowledgment cannot revive withdrawn/replaced identity',async t=>{
  const w=await wire(t);
  const old=w.owner.report(prompt()), a=await w.next();
  const current=w.owner.report(prompt('native-a','new name')), b=await w.next();
  await a.c.result(a.r,{}); await old; assert.equal(w.owner.admitted,null);
  await b.c.result(b.r,{}); await current; assert.equal(w.owner.admitted.name,'new name');
  const pending=w.owner.report(prompt('native-a','renamed')); const p=await w.next();
  await w.owner.report({hook_event_name:'SessionEnd',session_id:'native-a'});
  await assert.rejects(pending); assert.equal(w.owner.admitted,null); assert.equal(w.owner.ended,false);
  // The closed transport cannot restore presence even if an obsolete result was pending.
  assert.ok(p.c.signal.aborted || w.owner.connection===null);
});
test('hello acknowledgment admits before the immediately following daemon delivery',async t=>{
  let seen; const w=await wire(t,{deliver:async captured=>{seen=captured.session_id;return {disposition:'written'};}});
  const ready=w.owner.report(prompt()), {c,r}=await w.next();
  const ack=c.result(r,{});
  const delivered=c.call('message.deliver',{message_id:'m',from:{session_id:'sender',product:'test',groups:[]},body:'body'});
  await ack; await ready; assert.deepEqual(await delivered,{disposition:'written'}); assert.equal(seen,'native-a');
});
test('loss is terminal and pending public calls settle without redial',async t=>{
  const w=await wire(t); const {c}=await publish(w);
  const pending=w.owner.action('list',{}); const request=await w.next(); assert.equal(request.r.method,'session.list');
  c.close(); await assert.rejects(pending); await w.owner.connection?.done;
  await Promise.resolve(); assert.equal(w.owner.ended,true); assert.throws(()=>w.owner.report(prompt()),/not_connected/); assert.equal(w.count(),1);
});
test('supersession is terminal before reply success or failure; no unhandled rejection',async t=>{
  for(const fail of [false,true]) {
    const w=await wire(t); const {c}=await publish(w);
    if(fail) w.owner.connection.result=()=>Promise.reject(new Error('controlled output failure'));
    const superseded=c.call('session.superseded',{});
    if(fail) await assert.rejects(superseded); else await superseded;
    assert.equal(w.owner.reason,'superseded'); assert.equal(w.owner.ended,true);
    assert.throws(()=>w.owner.report(prompt()),/not_connected/); assert.equal(w.count(),1);
  }
});
test('invalid report shapes and launch metadata cannot fabricate presence',async t=>{
  const w=await wire(t);
  for(const e of [null,{},prompt(''),prompt('${session_id}'),{...prompt(),agent_id:'agent'}, {...prompt(),hook_event_name:'SubagentStart'},prompt('a','bad\nname')]) {
    assert.throws(()=>w.owner.report(e));
  }
  assert.equal(w.count(),0);
  assert.throws(()=>new Owner({env:{SESSIONBUS_LAUNCH_TOKEN:'token'}}),/unavailable/);
  const bad=new Owner({env:{SESSIONBUS_GROUPS:'["a","a"]'}}); assert.throws(()=>bad.report(prompt()),/wire/);
});
test('uncertain native transfer sends valid Internal on actual kit wire; daemon sender shape is no_receipt',async t=>{
  const {ProtocolError,validate}=await import('@sessionbus/kit');
  const w=await wire(t,{deliver:async()=>{throw new ProtocolError({code:-32603,message:'internal',data:'uncertain_submission'});}});
  const {c}=await publish(w);
  await assert.rejects(c.call('message.deliver',{message_id:'m',from:{session_id:'sender',product:'test',groups:[]},body:'x'}),e=>e.code===-32603&&e.data==='uncertain_submission');
  // This is the existing daemon Internal->no_receipt result shape, not a native receipt.
  assert.ok(validate('MessageSendResult',{message_id:'m',deliveries:[{target:'native-a',session_id:'native-a',delivery_id:'d',disposition:'rejected',reason:'no_receipt'}]}));
  assert.equal(w.owner.ended,false);
});
test('identity change cancels captured delivery and pending Caller work before new publication',async t=>{
  let entered;const entry=new Promise(r=>entered=r);
  const w=await wire(t,{deliver:ctx=>new Promise((resolve,reject)=>{
    entered(ctx);ctx.signal.addEventListener('abort',()=>reject(new Error('uncertain submission')),{once:true});
  })});
  const {c}=await publish(w); const pending=w.owner.action('list',{});await w.next();
  const transfer=c.call('message.deliver',{message_id:'m',from:{session_id:'s',product:'t',groups:[]},body:'x'});
  const captured=await entry;const rejected=assert.rejects(transfer);const cancelled=assert.rejects(pending);
  await publish(w,prompt('native-b'));await rejected;await cancelled;
  assert.equal(captured.session_id,'native-a');assert.ok(captured.signal.aborted);assert.equal(w.owner.admitted.session_id,'native-b');
});
test('cancelled Caller request loses local pending entry; actual late kit reply closes integration',async t=>{
  const w=await wire(t);await publish(w);const cancel=new AbortController();
  const pending=w.owner.action('list',{},cancel.signal);const {c,r}=await w.next();const local=w.owner.connection;
  cancel.abort();await assert.rejects(pending);assert.equal(local.pending.size,0);
  await c.result(r,{sessions:[]});await local.done;await Promise.resolve();
  assert.equal(w.owner.ended,true);assert.equal(w.owner.reason,'connection_lost');assert.equal(w.count(),1);
});
