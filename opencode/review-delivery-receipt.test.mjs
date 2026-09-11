// SPDX-License-Identifier: MIT

import test from 'node:test';
import assert from 'node:assert/strict';
import { NativeDelivery } from './delivery.mjs';
test('later FIFO failure does not rewrite an already written receipt',async()=>{
  const life=new AbortController();let release;const held=new Promise(r=>release=r);let queries=0,submits=0;
  const delivery=new NativeDelivery({sessionID:'ses_native',signal:life.signal,
    status:async()=>{if(++queries===1)await held;return 'idle';},info:async()=>({directory:'/work'}),
    submit:async(_p,_s,attempt)=>{if(++submits===2)throw Error('later entry unavailable');attempt();},reserve:()=>true,release:()=>{}});
  const message=n=>({message_id:`message_${n}`,body:'hello',from:{product:'test',session_id:'sender',groups:[]}});
  const first=delivery.enqueue(life.signal,message(1));
  const second=await delivery.enqueue(life.signal,message(2));
  release();const receipt=await first;
  try {assert.equal(second.disposition,'queued_for_next_turn');assert.equal(receipt.disposition,'written',JSON.stringify(receipt));}
  finally {life.abort();await delivery.dispose();}
});
