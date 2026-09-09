// SPDX-License-Identifier: MIT
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { PassThrough, Writable } from 'node:stream';
import { serve, hiddenTool } from './mcp.mjs';

function harness(owner) {
  const input=new PassThrough(), output=new PassThrough(), pending=[], received=[];
  let buffer='';output.on('data',b=>{buffer+=b;for(let n;(n=buffer.indexOf('\n'))>=0;){const x=JSON.parse(buffer.slice(0,n));buffer=buffer.slice(n+1);if(pending.length)pending.shift()(x);else received.push(x);}});
  const service=serve(owner,input,output);
  return {input,output,service,send:x=>input.write(JSON.stringify(x)+'\n'),next:()=>received.length?Promise.resolve(received.shift()):new Promise(r=>pending.push(r))};
}
const request=(id,method,params={})=>({jsonrpc:'2.0',id,method,params});
test('initialize, ping and catalog; hidden native-shaped call excluded from public actions',async()=>{
  const reports=[];const h=harness({report:async e=>reports.push(e),end(){}});
  h.send(request(1,'initialize',{protocolVersion:'2025-11-25'})); assert.equal((await h.next()).result.protocolVersion,'2025-11-25');
  h.send(request(2,'tools/list')); assert.deepEqual((await h.next()).result.tools.map(t=>t.name),['sessionbus']);
  const event={hook_event_name:'Stop',session_id:'actual-native',session_title:''};
  h.send(request(3,'tools/call',{name:hiddenTool,arguments:event})); assert.ok((await h.next()).result.content);assert.deepEqual(reports,[event]);
  h.send(request(4,'ping'));assert.deepEqual((await h.next()).result,{});h.service.close();
});
test('pending tool cannot block report, notification or EOF; late answer never writes after close',async()=>{
  let finish, markEnd; const ended=new Promise(r=>markEnd=r); const reports=[];
  const h=harness({action:()=>new Promise(r=>finish=r),report:async e=>reports.push(e),end(){markEnd();}});
  h.send(request(1,'tools/call',{name:'sessionbus',arguments:{action:'list',arguments:{}}}));
  h.send({jsonrpc:'2.0',method:'notifications/initialized'});
  h.send(request(2,'tools/call',{name:hiddenTool,arguments:{hook_event_name:'SessionEnd',session_id:'id'}}));
  assert.equal((await h.next()).id,2);assert.equal(reports.length,1);
  h.input.end();await ended;finish({sessions:[]});await Promise.resolve();h.output.destroy();
});
test('malformed frames, unknown methods, invalid public arguments and report errors are protocol replies',async()=>{
  const h=harness({end(){},report(){throw Error('unusable native identity report');}});
  h.input.write('{bad\n');assert.equal((await h.next()).error.code,-32700);
  h.send(request(1,'unknown'));assert.equal((await h.next()).error.code,-32601);
  h.send(request(2,'tools/call',{name:hiddenTool,arguments:{}}));assert.equal((await h.next()).result.isError,true);
  h.send(request(3,'tools/call',{name:'sessionbus',arguments:{action:hiddenTool,arguments:{}}}));assert.equal((await h.next()).result.isError,true);
  h.service.close();
});
test('output failure terminates owner and closes report processing',async()=>{
  let closed;const done=new Promise(r=>closed=r),input=new PassThrough();
  const output=new Writable({write(_b,_e,cb){cb(new Error('controlled output failure'));}});
  serve({end:closed},input,output);input.write(JSON.stringify(request(1,'ping'))+'\n');await done;input.destroy();
});
test('published stdio EOF closes actual exported Connection and pending Caller request',async t=>{
  const {createServer}=await import('node:net'); const {mkdtempSync,rmSync}=await import('node:fs');
  const {tmpdir}=await import('node:os'); const {join}=await import('node:path'); const {once}=await import('node:events');
  const {Connection}=await import('@sessionbus/kit'); const {Owner}=await import('./owner.mjs');
  const dir=mkdtempSync(join(tmpdir(),'claude-mcp-')),path=join(dir,'s');let peer, called;
  const pending=new Promise(r=>called=r);
  const server=createServer(s=>{peer=new Connection(s,false,req=>{
    if(req.method==='session.hello') void peer.result(req,{}); else called(req);
  });});server.listen(path);await once(server,'listening');
  const owner=new Owner({env:{SESSIONBUS_SOCKET:path}}),h=harness(owner);
  t.after(async()=>{h.service.close();peer?.close();await new Promise(r=>server.close(r));rmSync(dir,{recursive:true,force:true});});
  h.send(request(1,'tools/call',{name:hiddenTool,arguments:{hook_event_name:'Stop',session_id:'reported'}}));
  await h.next();assert.equal(owner.admitted.session_id,'reported');
  h.send(request(2,'tools/call',{name:'sessionbus',arguments:{action:'list',arguments:{}}}));
  assert.equal((await pending).method,'session.list');const connection=owner.connection;
  h.input.end();await connection.done;await peer.done;
  assert.equal(owner.ended,true);assert.equal(connection.pending.size,0);assert.equal(owner.admitted,null);
});
test('MCP cancellation targets only the pending public request and removes it on completion',async()=>{
  const calls=[];let finish;
  const h=harness({end(){},action(_a,_v,signal){calls.push(signal);return new Promise((resolve,reject)=>{finish=resolve;signal.addEventListener('abort',()=>reject(signal.reason),{once:true});});}});
  h.send(request(10,'tools/call',{name:'sessionbus',arguments:{action:'list',arguments:{}}}));
  h.send({jsonrpc:'2.0',method:'notifications/cancelled',params:{requestId:99}});assert.equal(calls[0].aborted,false);
  h.send({jsonrpc:'2.0',method:'notifications/cancelled',params:{requestId:10}});assert.equal(calls[0].aborted,true);
  h.send(request(11,'ping'));assert.equal((await h.next()).id,11);finish({sessions:[]});await Promise.resolve();
  h.send(request(12,'tools/call',{name:'sessionbus',arguments:{action:'list',arguments:{}}}));finish({sessions:[]});
  assert.equal((await h.next()).id,12);
  h.send({jsonrpc:'2.0',method:'notifications/cancelled',params:{requestId:12}});assert.equal(calls[1].aborted,false);
  h.service.close();
});

for (const collect of ['status','wait']) test(`cancelled MCP wait leaves its result collectible by ${collect}`,async t=>{
  const {createServer}=await import('node:net');const {mkdtempSync,rmSync}=await import('node:fs');
  const {tmpdir}=await import('node:os');const {join}=await import('node:path');const {once}=await import('node:events');
  const {Connection}=await import('@sessionbus/kit');const {Owner}=await import('./owner.mjs');
  const dir=mkdtempSync(join(tmpdir(),'claude-wait-')),path=join(dir,'s');const queue=[],readers=[];let peer;
  const server=createServer(stream=>{peer=new Connection(stream,false,r=>{
    if(r.method==='session.hello') void peer.result(r,{});
    else if(readers.length) readers.shift()(r);else queue.push(r);
  });});server.listen(path);await once(server,'listening');
  const next=()=>queue.length?Promise.resolve(queue.shift()):new Promise(r=>readers.push(r));
  const owner=new Owner({env:{SESSIONBUS_SOCKET:path}}),h=harness(owner);
  t.after(async()=>{h.service.close();peer?.close();await new Promise(r=>server.close(r));rmSync(dir,{recursive:true,force:true});});
  const call=(id,action,args)=>h.send(request(id,'tools/call',{name:'sessionbus',arguments:{action,arguments:args}}));
  const value=frame=>JSON.parse(frame.result.content[0].text);
  h.send(request(1,'tools/call',{name:hiddenTool,arguments:{hook_event_name:'Stop',session_id:'reported'}}));await h.next();
  call(2,'start',{session_id:'daemon-lane',input:'work'});const {turn_id}=value(await h.next());const run=await next();assert.equal(run.method,'turn.run');
  call(3,'wait',{turn_id});h.send(request(4,'ping'));assert.equal((await h.next()).id,4);
  h.send({jsonrpc:'2.0',method:'notifications/cancelled',params:{requestId:3}});
  h.send(request(5,'ping'));assert.equal((await h.next()).id,5);
  // Ordered actual Connection replies provide a public completion barrier.
  await peer.result(run,{outcome:'completed',result:'keep-this-result'});
  call(6,'list',{});const list=await next();assert.equal(list.method,'session.list');await peer.result(list,{sessions:[]});assert.equal((await h.next()).id,6);
  call(7,collect,{turn_id});const collected=await h.next();assert.equal(collected.id,7);
  assert.deepEqual(value(collected),{turn_id,session_id:'daemon-lane',state:'done',result:{outcome:'completed',result:'keep-this-result'}});
  call(8,'status',{turn_id});const consumed=await h.next();assert.equal(consumed.id,8);assert.equal(consumed.result.isError,true);assert.match(consumed.result.content[0].text,/unknown_turn/);
});

test('fulfilled consuming operation keeps its MCP response when cancellation arrives afterwards',async()=>{
  const {Caller,Connection}=await import('@sessionbus/kit');
  const {Duplex}=await import('node:stream');
  let clientStream,serverStream;
  clientStream=new Duplex({read(){},write(b,_e,cb){serverStream.push(b);cb();}});
  serverStream=new Duplex({read(){},write(b,_e,cb){clientStream.push(b);cb();}});
  let complete;const requested=new Promise(r=>complete=r);
  const server=new Connection(serverStream,false,r=>complete(r));const client=new Connection(clientStream,true);
  const caller=new Caller(client);let h;
  const owner={end(){caller.disconnected();client.close();server.close();},action:async(action,args,signal)=>{
    const value=await caller.action(action,args,signal);
    if(action==='wait') h.send({jsonrpc:'2.0',method:'notifications/cancelled',params:{requestId:2}});
    return value;
  }};
  h=harness(owner);
  h.send(request(1,'tools/call',{name:'sessionbus',arguments:{action:'start',arguments:{session_id:'lane',input:'work'}}}));
  const {turn_id}=JSON.parse((await h.next()).result.content[0].text);const run=await requested;
  h.send(request(2,'tools/call',{name:'sessionbus',arguments:{action:'wait',arguments:{turn_id}}}));
  await server.result(run,{outcome:'completed',result:'completion-won'});
  const frame=await h.next();assert.equal(frame.id,2);assert.equal(JSON.parse(frame.result.content[0].text).result.result,'completion-won');
  assert.throws(()=>caller.status({turn_id}),/unknown_turn/);h.service.close();
});
