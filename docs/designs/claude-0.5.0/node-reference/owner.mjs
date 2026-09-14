// SPDX-License-Identifier: MIT
import { resolve } from 'node:path';

// Same resolution as b1d7adb sdk/go/internal/stateroot, with one dial only.
export function socketPath(env = process.env, cwd = process.cwd(), uid = process.getuid()) {
  return resolve(cwd, env.SESSIONBUS_SOCKET || (env.XDG_RUNTIME_DIR
    ? `${env.XDG_RUNTIME_DIR}/sessionbus/presence.sock`
    : `/tmp/sessionbus-${uid}/presence.sock`));
}

import { connect } from 'node:net';
import { Connection, Caller, ProtocolError, validate } from '@sessionbus/kit';
import { deliverNative } from './delivery.mjs';

const missing = value => value === undefined || value === '' || /^\$\{[^}]+\}$/.test(value);
const unavailable = () => new ProtocolError({ code:-32002, message:'not_connected' });

export class Owner {
  constructor({ env = process.env, dial = connect, deliver = deliverNative } = {}) {
    if (env.SESSIONBUS_LAUNCH_TOKEN) throw new Error('Claude lane mode is unavailable in this interactive candidate');
    try { this.groups = JSON.parse(env.SESSIONBUS_GROUPS || '[]'); }
    catch { throw new Error('invalid launch groups JSON'); }
    this.socket = socketPath(env);
    this.nativeSocket = env.CLAUDE_CODE_MESSAGING_SOCKET;
    this.dial = dial; this.deliver = deliver;
    this.generation = 0; this.ended = false;
  }

  report(event) {
    if (this.ended) throw unavailable();
    if (!event || !['UserPromptSubmit','Stop','SessionEnd'].includes(event.hook_event_name) ||
        !missing(event.agent_id) || typeof event.session_id !== 'string' || missing(event.session_id)) {
      throw new Error('unusable native identity report');
    }
    const id = event.session_id;
    const name = missing(event.session_title) ? (this.identity?.session_id === id ? this.identity.name : undefined) : event.session_title;
    const identity = { protocol:1, product:'claude-peer', session_id:id, groups:this.groups, info:{}, ...(name === undefined ? {} : {name}) };
    if (!validate('SessionHelloRequest',identity)) throw new Error('native identity or launch groups violate the Sessionbus wire');
    if (event.hook_event_name === 'SessionEnd') {
      if (this.identity?.session_id === id) this.withdraw();
      return Promise.resolve();
    }
    if (this.admitted && this.identity.session_id === id && this.identity.name === name) return Promise.resolve();
    if (this.identity?.session_id !== id) {
      this.withdraw();
      this.context = new AbortController();
    }
    this.identity = identity; this.admitted = null;
    const generation = ++this.generation;
    const c = this.connection || this.open();
    return c.call('session.hello', identity, undefined, () => {
      // Runs inside the kit receive path, before the next daemon request.
      if (!this.ended && this.connection === c && this.generation === generation) this.admitted = identity;
    }).catch(error => {
      if (this.connection === c && !this.ended) this.end('hello_failed');
      throw error;
    });
  }

  open() {
    const c = new Connection(this.dial(this.socket), true, request => this.handle(c, request));
    this.connection = c; this.caller = new Caller(c);
    void c.done.then(() => { if (this.connection === c && !this.ended) this.end('connection_lost'); });
    return c;
  }

  withdraw() {
    ++this.generation; this.admitted = null; this.identity = null;
    this.context?.abort(unavailable()); this.caller?.disconnected();
    const c = this.connection; this.connection = null; this.caller = null;
    c?.close();
  }

  end(reason = 'closed') {
    if (this.ended) return;
    this.ended = true; this.reason = reason; this.withdraw();
  }

  action(action, args, signal) {
    if (this.ended || !this.admitted || !this.connection || this.context.signal.aborted) throw unavailable();
    const cancel = signal ? AbortSignal.any([signal,this.context.signal]) : this.context.signal;
    cancel.throwIfAborted();
    return this.caller.action(action, args, cancel);
  }

  handle(c, request) {
    if (this.connection !== c) { c.close(); return; }
    if (request.method === 'session.superseded') {
      // Classify terminal intent before replying; either reply outcome closes.
      this.ended = true; this.reason = 'superseded';
      ++this.generation; this.admitted = null; this.context?.abort(unavailable()); this.caller?.disconnected();
      let reply;
      try { reply = c.result(request, {}); } catch { this.withdraw(); return; }
      void reply.then(() => this.withdraw(), () => this.withdraw());
      return;
    }
    const captured = this.connection === c && this.admitted && !this.ended
      ? { session_id:this.admitted.session_id, socket:this.nativeSocket, signal:this.context.signal } : null;
    void Promise.resolve().then(() => {
      if (request.method !== 'message.deliver') return c.error(request,-32600);
      if (!captured) return c.result(request,{disposition:'rejected',reason:'unpublished_before_submission'});
      return this.deliver(captured, request.params).then(result => c.result(request,result), error =>
        c.error(request,-32603,error instanceof ProtocolError ? error.data : 'uncertain_submission'));
    }).catch(() => { c.close(); });
  }
}
