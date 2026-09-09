// SPDX-License-Identifier: MIT
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:net';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { once } from 'node:events';
import { Connection, ACTIONS } from '@sessionbus/kit';
import { Owner } from './owner.mjs';
import { callTool, tool } from './tools.mjs';

async function wire(t) {
  const dir=mkdtempSync(join(tmpdir(),'claude-actions-')), path=join(dir,'s');
  const queued=[], readers=[], seen=[], connections=[];
  const server=createServer(stream=>{
    const c=new Connection(stream,false,r=>{
      const item={c,r};seen.push(item);if(readers.length) readers.shift()(item);else queued.push(item);
    });connections.push(c);
  });server.listen(path);await once(server,'listening');
  const owner=new Owner({env:{SESSIONBUS_SOCKET:path}});
  t.after(async()=>{owner.end();connections.forEach(c=>c.close());await new Promise(r=>server.close(r));rmSync(dir,{recursive:true,force:true});});
  const next=()=>queued.length?Promise.resolve(queued.shift()):new Promise(r=>readers.push(r));
  const ready=owner.report({hook_event_name:'UserPromptSubmit',session_id:'native-owner'});
  const {c,r}=await next();assert.equal(r.method,'session.hello');await c.result(r,{});await ready;
  return {owner,next,seen,call:(action,args)=>callTool(owner,{action,arguments:args})};
}

test('single public catalog includes usable action fields but no hidden identity action',()=>{
  assert.equal(tool.name,'sessionbus');assert.deepEqual(tool.inputSchema.properties.action.enum,[...ACTIONS]);
  assert.ok(!JSON.stringify(tool).includes('native_identity_event'));
  for(const field of ['message','target','targets','group','open','resume_session_id','turn_id','timeout_ms']) assert.ok(tool.description.includes(field));
  for(const value of [{action:'native_identity_event',arguments:{}},{action:'list'},{action:'list',arguments:[],extra:true}]) assert.throws(()=>callTool({},value));
});

test('public peer/group sends and lane caller operations traverse actual Connection validation',async t=>{
  const w=await wire(t);
  const receipt={message_id:'message-id',deliveries:[{target:'peer@host',session_id:'peer@host',delivery_id:'delivery-id',disposition:'written'}]};
  const cases=[
    ['list','session.list',{}, {sessions:[]}],
    ['send','message.send',{target:'peer@host',message:'peer message'},receipt],
    ['send','message.send',{group:'review',host:'host',message:'group message'},receipt],
    ['send','message.send',{targets:['peer@host'],message:'explicit targets'},receipt],
    ['describe','lane.describe',{product:'other-product'},{product:'other-product',supported_open_fields:['cwd','arguments'],extra_arguments:[]}],
    ['spawn','lane.spawn',{product:'other-product',name:'lane',open:{cwd:'/work',arguments:['--native','value']}},{session_id:'daemon-lane'}],
    ['spawn','lane.spawn',{resume_session_id:'daemon-lane'},{session_id:'daemon-lane'}],
    ['run','turn.run',{session_id:'daemon-lane',input:'one'},{outcome:'completed',result:'answer'}],
    ['interrupt','turn.interrupt',{session_id:'daemon-lane'},{}],
    ['close','session.close',{session_id:'daemon-lane'},{}],
    ['forget','session.close',{session_id:'daemon-lane'},{}]
  ];
  for(const [action,method,args,result] of cases) {
    const pending=w.call(action,args);const {c,r}=await w.next();
    assert.equal(r.method,method);assert.deepEqual(r.params,action==='forget'?{...args,forget:true}:args);
    await c.result(r,result);assert.deepEqual(await pending,result);
  }
  for(const [action,args] of [['send',{to:'peer',body:'invalid'}],['send',{target:'peer',group:'review',message:'invalid'}],['spawn',{product:'other-product',name:'missing-open'}]]) {
    const before=w.seen.length;await assert.rejects(async()=>w.call(action,args));
    const pending=w.call('list',{});const {c,r}=await w.next();assert.equal(r.method,'session.list');await c.result(r,{sessions:[]});await pending;
    assert.equal(w.seen.length,before+1,'invalid arguments must not reach the wire');
  }
});

test('real Caller start/status/wait collects a validated run result once',async t=>{
  const w=await wire(t);const {turn_id}=await w.call('start',{session_id:'daemon-lane',input:'work'});
  const {c,r}=await w.next();assert.equal(r.method,'turn.run');assert.deepEqual(r.params,{session_id:'daemon-lane',input:'work'});
  assert.deepEqual(await w.call('status',{turn_id}),{turn_id,session_id:'daemon-lane',state:'running'});
  const waiting=w.call('wait',{turn_id});await c.result(r,{outcome:'completed',result:'retained-answer'});
  assert.deepEqual(await waiting,{turn_id,session_id:'daemon-lane',state:'done',result:{outcome:'completed',result:'retained-answer'}});
  await assert.rejects(async()=>w.call('status',{turn_id}),/unknown_turn/);
});

test('real Caller wait settles on owner disconnect without an adapter result cache',async t=>{
  const w=await wire(t);const {turn_id}=await w.call('start',{session_id:'daemon-lane',input:'work'});await w.next();
  const waiting=w.call('wait',{turn_id});await Promise.resolve();w.owner.end();
  assert.deepEqual(await waiting,{turn_id,session_id:'daemon-lane',state:'unavailable',reason:'-32002 not_connected'});
});
