// SPDX-License-Identifier: MIT

import test from 'node:test';
import assert from 'node:assert/strict';
import { NativeDelivery } from './delivery.mjs';

test('native idle arriving during status await remains a drain trigger', async () => {
  const owner = new AbortController();
  let delivery, idleWork, queries=0, submissions=0, bytes=0;
  delivery = new NativeDelivery({
    sessionID:'ses_native', signal:owner.signal,
    status:()=>{
      queries++;
      if (queries===1) {
        queueMicrotask(()=>{ idleWork=delivery.idle(); });
        return Promise.resolve('busy');
      }
      return Promise.resolve('idle');
    },
    info:async()=>({directory:'/work'}),
    submit:async(_params,_signal,submitted)=>{submitted(); submissions++;},
    reserve:n=>{bytes+=n;return true;}, release:n=>{bytes-=n;},
  });
  const receipt=await delivery.enqueue(owner.signal,{message_id:'message_one',body:'hello',from:{product:'test',session_id:'sender',groups:[]}});
  await idleWork;
  try { assert.equal(submissions,1,`idle lost: receipt=${JSON.stringify(receipt)} queries=${queries} bytes=${bytes}`); }
  finally { owner.abort(); await delivery.dispose(); }
});
