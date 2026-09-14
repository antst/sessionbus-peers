// SPDX-License-Identifier: MIT
import { ACTIONS } from '@sessionbus/kit';

export const tool = {
  name:'sessionbus', description:`Call Sessionbus using the current published Claude identity. Written means local write completion only.
Arguments by action (wire validation remains in the public kit):
list: {} or {session_id:string} or {host:string}; session_id and host are exclusive.
send: {message:nonempty string, target:string} or {message, targets:unique nonempty string[]} or {message, group:string, host?:string}; choose exactly one addressing form; host applies only to group. Use returned session IDs or unambiguous names.
describe: {product:string, host?:string}; returns supported open fields and native extra arguments.
spawn fresh: {product:string, name:string, open:object, host?:string, extra_groups?:unique string[]}. open accepts cwd, permission_mode, model, reasoning_effort (strings), arguments (string[]); use describe to check product support. spawn resume: {resume_session_id:string} alone. This candidate cannot provide Claude lanes.
run: {session_id:string, input:nonempty string}; waits for the terminal result.
start: same fields as run; returns a local turn_id for collection.
status: {turn_id:string}; running is non-consuming; a completed/unavailable result is consumed once.
wait: {turn_id:string, timeout_ms?:nonnegative integer}; collects the result or returns running at the explicit bound. Cancelling a pending wait leaves its run/result collectible; it does not interrupt the remote turn. Handles belong to this MCP owner, not daemon session IDs.
interrupt: {session_id:string}; acknowledgment is not terminal completion.
close: {session_id:string, forget?:boolean}; forget: {session_id:string} closes with forget=true.
No unlisted argument fields. Message and input strings have a 262144-character wire limit. Native policy can deny any public call.`,
  inputSchema:{type:'object',required:['action','arguments'],additionalProperties:false,properties:{
    action:{type:'string',enum:[...ACTIONS]}, arguments:{type:'object'}
  }}
};
export function callTool(owner, input, signal) {
  if (!input || Object.keys(input).some(k=>!['action','arguments'].includes(k)) ||
      !ACTIONS.includes(input.action) || !input.arguments || typeof input.arguments !== 'object' || Array.isArray(input.arguments)) {
    throw new Error('expected a Sessionbus action and arguments object');
  }
  signal?.throwIfAborted();
  // The public Caller action path passes this signal to cancellable wait.
  // Do not race a consuming result or keep an adapter result cache.
  return owner.action(input.action,input.arguments,signal);
}
