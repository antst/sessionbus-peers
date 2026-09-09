#!/usr/bin/env node
// SPDX-License-Identifier: MIT
import { accessSync, constants, realpathSync, statSync } from 'node:fs';
import { resolve, delimiter } from 'node:path';
import { fileURLToPath } from 'node:url';
import { socketPath } from './owner.mjs';

export function launchPlan(args, env = process.env, cwd = process.cwd()) {
  if (env.SESSIONBUS_LAUNCH_TOKEN) throw new Error('Claude lane mode is unavailable in this interactive candidate');
  const suffix = args.length >= 2 && args.at(-2) === '-g';
  const value = suffix ? args.at(-1) : '';
  return { args: suffix ? args.slice(0, -2) : [...args], env: { ...env,
    SESSIONBUS_GROUPS: JSON.stringify(value === '' ? [] : value.split(',')),
    SESSIONBUS_SOCKET: socketPath(env, cwd) } };
}

export function nativePath(env = process.env, cwd = process.cwd()) {
  for (const dir of (env.PATH || '').split(delimiter)) {
    const path = resolve(cwd, dir, 'claude');
    try { accessSync(path, constants.X_OK); if (statSync(path).isFile()) return path; } catch {}
  }
  throw new Error('claude executable was not found on PATH');
}

export function main(args = process.argv.slice(2)) {
  const plan = launchPlan(args);
  if (typeof process.execve !== 'function') throw new Error('Node with process.execve is required');
  const path = nativePath(plan.env);
  process.execve(path, [path, ...plan.args], plan.env);
}

if (process.argv[1] && realpathSync(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { main(); } catch (error) { process.stderr.write(`claude-peer: ${error.message}\n`); process.exitCode = 1; }
}
