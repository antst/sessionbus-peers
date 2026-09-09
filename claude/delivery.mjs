// SPDX-License-Identifier: MIT
import { connect } from 'node:net';
import { ProtocolError } from '@sessionbus/kit';

export function deliverNative(context, request, dial = connect) {
  const { session_id, socket: path, signal } = context;
  const rejected = reason => ({ disposition:'rejected', reason });
  if (signal.aborted) return Promise.resolve(rejected('identity_unavailable_before_submission'));
  if (!path) return Promise.resolve(rejected('native_socket_missing_before_submission'));
  const frame = { msgV:1, msg_id:request.message_id, type:'user', priority:'next',
    from:request.from.name ?? request.from.session_id, session_id,
    message:{ role:'user', content:request.body } };
  return new Promise((resolve, reject) => {
    let socket, submitted = false, settled = false;
    const finish = (ok, reason) => {
      if (settled) return;
      settled = true; signal.removeEventListener('abort', abort); socket?.destroy();
      if (ok) resolve({disposition:'written'});
      else if (submitted) reject(new ProtocolError({code:-32603,message:'internal',data:'uncertain_submission'}));
      else resolve(rejected(reason));
    };
    const abort = () => finish(false,'identity_unavailable_before_submission');
    signal.addEventListener('abort', abort, {once:true});
    try {
      socket = dial(path);
      socket.once('error', () => finish(false,'native_connect_failed_before_submission'));
      socket.once('close', () => finish(false,'native_closed_before_submission'));
      socket.once('end', () => finish(false,'native_eof_before_submission'));
      socket.once('connect', () => {
        if (settled) return;
        if (signal.aborted) { abort(); return; }
        submitted = true;
        try { socket.write(`${JSON.stringify(frame)}\n`, error => finish(!error)); }
        catch { finish(false); }
      });
    } catch { finish(false,'native_connect_failed_before_submission'); }
  });
}
