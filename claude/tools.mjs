// SPDX-License-Identifier: MIT
import { ACTIONS } from '@sessionbus/kit';

export const tool = {
  name:'sessionbus', description:'Call Sessionbus using the current published Claude identity. Written means local write completion only.',
  inputSchema:{type:'object',required:['action','arguments'],additionalProperties:false,properties:{
    action:{type:'string',enum:[...ACTIONS]}, arguments:{type:'object'}
  }}
};
export function callTool(owner, input, signal) {
  if (!input || Object.keys(input).some(k=>!['action','arguments'].includes(k)) ||
      !ACTIONS.includes(input.action) || !input.arguments || typeof input.arguments !== 'object' || Array.isArray(input.arguments)) {
    throw new Error('expected a Sessionbus action and arguments object');
  }
  return owner.action(input.action,input.arguments,signal);
}
