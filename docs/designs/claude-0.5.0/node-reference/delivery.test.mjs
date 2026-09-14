// SPDX-License-Identifier: MIT
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { EventEmitter, once } from 'node:events';
import { createServer } from 'node:net';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { ProtocolError, validate } from '@sessionbus/kit';
import { deliverNative } from './delivery.mjs';

const request={message_id:'original',from:{session_id:'sender@host',product:'test',groups:['g']},body:'unaltered\nbody'};
function fixture() {
  const socket=new EventEmitter(); let callback, bytes, destroyed=false;
  socket.write=(b,cb)=>{bytes=b;callback=cb;}; socket.destroy=()=>{destroyed=true;};
  const controller=new AbortController(); const context={session_id:'old-id',socket:'/test',signal:controller.signal};
  return {socket,controller,context,bytes:()=>bytes,callback:e=>callback(e),destroyed:()=>destroyed};
}
test('one actual local socket write returns written without waiting for native EOF',async t=>{
  const dir=mkdtempSync(join(tmpdir(),'claude-delivery-')),path=join(dir,'s');
  let receive; const received=new Promise(r=>receive=r);
  const server=createServer(s=>s.once('data',b=>{receive(JSON.parse(b));s.destroy();}));
  server.listen(path); await once(server,'listening');
  t.after(async()=>{await new Promise(r=>server.close(r));rmSync(dir,{recursive:true,force:true});});
  const result=await deliverNative({session_id:'actual',socket:path,signal:new AbortController().signal},request);
  assert.deepEqual(result,{disposition:'written'}); const frame=await received;
  assert.deepEqual(frame,{msgV:1,msg_id:'original',type:'user',priority:'next',from:'sender@host',session_id:'actual',message:{role:'user',content:request.body}});
});
test('captured target/source remain fixed across delayed write completion and rename',async()=>{
  const f=fixture(); const result=deliverNative(f.context,{...request,from:{...request.from,name:'Real sender'}},()=>f.socket);
  f.context.session_id='new-id'; f.socket.emit('connect');
  assert.equal(JSON.parse(f.bytes()).session_id,'old-id'); assert.equal(JSON.parse(f.bytes()).from,'Real sender');
  f.callback(); assert.deepEqual(await result,{disposition:'written'}); assert.ok(f.destroyed());
});
test('missing socket, connect error and cancellation before submission are truthful rejections',async()=>{
  for(const kind of ['missing','connect','cancel']) {
    const f=fixture(); if(kind==='missing') f.context.socket='';
    const result=deliverNative(f.context,request,()=>f.socket);
    if(kind==='connect') f.socket.emit('error',new Error('controlled')); if(kind==='cancel') f.controller.abort();
    assert.equal((await result).disposition,'rejected'); assert.equal(f.bytes(),undefined);
  }
});
test('partial write, callback error, clear/supersession cancellation and EOF are uncertain after submission',async()=>{
  for(const event of ['callback','abort','end','close','error']) {
    const f=fixture(), result=deliverNative(f.context,request,()=>f.socket); f.socket.emit('connect');
    if(event==='callback') f.callback(new Error('partial')); else if(event==='abort') f.controller.abort(); else f.socket.emit(event,new Error('transport'));
    await assert.rejects(result,e=>e instanceof ProtocolError && e.code===-32603 && e.data==='uncertain_submission' && validate('RPCError',{code:e.code,message:e.message,data:e.data}));
    assert.ok(f.destroyed());
  }
});
test('complete write is not reclassified by a subsequent identity cancellation or EOF',async()=>{
  const f=fixture(), result=deliverNative(f.context,request,()=>f.socket); f.socket.emit('connect'); f.callback(); f.controller.abort(); f.socket.emit('end');
  assert.deepEqual(await result,{disposition:'written'});
  assert.ok(validate('MessageSendResult',{message_id:'m',deliveries:[{target:'target',session_id:'target',delivery_id:'d',disposition:'written'}]}));
  assert.ok(validate('MessageSendResult',{message_id:'m',deliveries:[{target:'target',session_id:'target',delivery_id:'d',disposition:'rejected',reason:'no_receipt'}]}));
});
