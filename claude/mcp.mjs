#!/usr/bin/env node
// SPDX-License-Identifier: MIT
import { realpathSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { Owner } from './owner.mjs';
import { createInterface } from 'node:readline';
import { ProtocolError } from '@sessionbus/kit';
import { callTool, tool } from './tools.mjs';

export const hiddenTool = 'native_identity_event';
const content = value => ({content:[{type:'text',text:JSON.stringify(value)}]});

// A pending public call never serializes stdin report/EOF processing.
export function serve(owner, input, output) {
  const lines = createInterface({input,crlfDelay:Infinity});
  let stopped = false;
  const pending = new Map();
  const stop = () => { if (!stopped) { stopped=true; owner.end('stdio_closed');
      for(const controller of pending.values()) controller.abort();
      pending.clear(); lines.close(); input.pause(); } };
  const write = frame => {
    if (stopped) return;
    try { output.write(`${JSON.stringify(frame)}\n`, error => { if(error) stop(); }); } catch { stop(); }
  };
  const error = (id,code,message) => write({jsonrpc:'2.0',id,error:{code,message}});
  async function dispatch(line) {
    let frame;
    try { frame=JSON.parse(line); } catch { error(null,-32700,'Parse error'); return; }
    const id=frame?.id;
    if (!frame || frame.jsonrpc!=='2.0' || typeof frame.method!=='string' ||
        (id!==undefined && typeof id!=='string' && !Number.isSafeInteger(id))) {
      error(typeof id==='string'||Number.isSafeInteger(id)?id:null,-32600,'Invalid Request'); return;
    }
    if(id===undefined) {
      if(frame.method==='notifications/cancelled') pending.get(frame.params?.requestId)?.abort();
      return; // Notifications never carry identity authority.
    }
    const p=frame.params ?? {};
    if(typeof p!=='object' || Array.isArray(p)) {error(id,-32602,'Invalid parameters');return;}
    let result;
    if(frame.method==='initialize') {
      if(typeof p.protocolVersion!=='string') {error(id,-32602,'Invalid initialize parameters');return;}
      result={protocolVersion:p.protocolVersion,capabilities:{tools:{}},serverInfo:{name:'sessionbus',version:'0.5.0-interactive.0'}};
    } else if(frame.method==='ping') result={};
    else if(frame.method==='tools/list') result={tools:[tool]};
    else if(frame.method==='tools/call') {
      if(![hiddenTool,tool.name].includes(p.name)) {error(id,-32602,'Unknown tool');return;}
      if(pending.has(id)) {error(id,-32600,'Request ID already in flight');return;}
      const controller=p.name===tool.name ? new AbortController() : null;
      if(controller) pending.set(id,controller);
      try {
        const value=controller ? await callTool(owner,p.arguments,controller.signal) : await owner.report(p.arguments);
        result=content(value ?? {});
      } catch(e) {
        if (controller?.signal.aborted && e === controller.signal.reason) return;
        result={...content(e instanceof ProtocolError ? {code:e.code,message:e.message,...(e.data===undefined?{}:{data:e.data})} : {error:e.message}),isError:true};
      } finally { if(controller) pending.delete(id); }
      // A fulfilled consuming operation won settlement; never drop its result
      // because a cancellation notification arrived after that completion.
    } else {error(id,-32601,'Method not found');return;}
    write({jsonrpc:'2.0',id,result});
  }
  lines.on('line',line=>{void dispatch(line).catch(stop);});
  lines.on('close',stop); input.on('error',stop); output.on('error',stop); output.on('close',stop);
  return {close:stop};
}

if (process.argv[1] && realpathSync(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { serve(new Owner(), process.stdin, process.stdout); }
  catch (error) { process.stderr.write(`sessionbus: ${error.message}\n`); process.exitCode = 1; }
}
