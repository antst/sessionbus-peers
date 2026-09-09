// SPDX-License-Identifier: MIT
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { Caller, ACTIONS } from '@sessionbus/kit';
import { callTool, tool } from './tools.mjs';

test('single public catalog exposes only published Caller actions',()=>{
  assert.equal(tool.name,'sessionbus'); assert.deepEqual(tool.inputSchema.properties.action.enum,[...ACTIONS]);
  assert.ok(!JSON.stringify(tool).includes('native_identity_event'));
  for(const value of [{action:'native_identity_event',arguments:{}},{action:'list'}, {action:'list',arguments:[],extra:true}])
    assert.throws(()=>callTool({},value));
});
test('public Caller forwarding preserves actual action parameters and results',async()=>{
  const seen=[]; const connection={call:async(method,args)=>{seen.push({method,args});return {native:'opaque-result'};}};
  const caller=new Caller(connection), owner={action:(...args)=>caller.action(...args)};
  for(const [action,method,args] of [['list','session.list',{}],['send','message.send',{to:'exact@host',body:'body'}],
    ['spawn','lane.spawn',{product:'other-product',name:'lane'}],['run','turn.run',{session_id:'daemon-id',input:'one'}],
    ['interrupt','turn.interrupt',{session_id:'daemon-id'}],['forget','session.close',{session_id:'daemon-id'}]]) {
    assert.deepEqual(await callTool(owner,{action,arguments:args}),{native:'opaque-result'});
    assert.deepEqual(seen.at(-1),{method,args:action==='forget'?{...args,forget:true}:args});
  }
});
test('Caller start/wait handles settle on integration disconnect without extra adapter state',async()=>{
  let resolve; const caller=new Caller({call:()=>new Promise(r=>{resolve=r;})});
  const {turn_id}=caller.start({session_id:'native-lane',input:'work'});
  const waiting=caller.wait({turn_id}); caller.disconnected();
  assert.deepEqual(await waiting,{turn_id,session_id:'native-lane',state:'unavailable',reason:'-32002 not_connected'});
  await Promise.resolve(); resolve({outcome:'completed',result:'late'});
});
