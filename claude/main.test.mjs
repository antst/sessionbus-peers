// SPDX-License-Identifier: MIT
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { launchPlan, nativePath } from './main.mjs';
import { socketPath } from './owner.mjs';

const main = fileURLToPath(new URL('./main.mjs', import.meta.url));
test('only terminal group pair is consumed; all native argv stays byte-exact', () => {
  for (const args of [[], ['-n',''], ['--resume','name','--resume','other'], ['--','-g'],
    ['mcp'], ['--bad=value'], ['--model','-g','prompt','extra'], ['-g','a','--version']]) {
    const p = launchPlan(args, { PATH:'/bin', SESSIONBUS_GROUPS:'["old"]' });
    assert.deepEqual(p.args, args); assert.equal(p.env.SESSIONBUS_GROUPS, '[]');
  }
  for (const [value, groups] of [['',[]], ['a,b',['a','b']], [' a,,a,',[' a','','a','']]]) {
    const p = launchPlan(['--','-n','native','-g',value], {});
    assert.deepEqual(p.args, ['--','-n','native']); assert.deepEqual(JSON.parse(p.env.SESSIONBUS_GROUPS), groups);
  }
  assert.throws(() => launchPlan([], { SESSIONBUS_LAUNCH_TOKEN:'test' }), /lane mode is unavailable/);
});
test('socket resolution matches Go stateroot precedence and absolute cleaning', () => {
  for (const [env, expected] of [[{},'/tmp/sessionbus-42/presence.sock'],
    [{ XDG_RUNTIME_DIR:'run/../r' },'/cwd/r/sessionbus/presence.sock'],
    [{ SESSIONBUS_SOCKET:'./s', XDG_RUNTIME_DIR:'/ignore' },'/cwd/s'],
    [{ SESSIONBUS_SOCKET:'', XDG_RUNTIME_DIR:'' },'/tmp/sessionbus-42/presence.sock']]) {
    assert.equal(socketPath(env, '/cwd', 42), expected);
  }
});
test('real exec preserves PID, argv, cwd, stdin/stdout/stderr, exit and signal', async t => {
  const dir = mkdtempSync(join(tmpdir(), 'claude-exec-')); t.after(() => rmSync(dir,{recursive:true,force:true}));
  const fixture = join(dir,'claude');
  writeFileSync(fixture, `#!${process.execPath}\nprocess.stdin.once('data', b => {
    process.stdout.write(JSON.stringify({pid:process.pid,cwd:process.cwd(),args:process.argv.slice(2),input:b.toString(),groups:process.env.SESSIONBUS_GROUPS}));
    process.stderr.write('fixture-error');
    if (process.argv.includes('signal')) process.kill(process.pid,'SIGTERM'); else process.exitCode=17;
  });\n`, {mode:0o755});
  assert.equal(nativePath({PATH:dir}),fixture);
  for (const mode of ['exit','signal']) {
    const p = spawn(process.execPath,[main,'-n','exact','--',mode,'-g','a,b'],{cwd:dir,env:{...process.env,PATH:dir,SESSIONBUS_LAUNCH_TOKEN:''}});
    let out='',err=''; p.stdout.on('data',b=>out+=b); p.stderr.on('data',b=>err+=b);
    const closed=once(p,'close'); p.stdin.end('fixture-input'); const [code,signal]=await closed;
    assert.deepEqual(JSON.parse(out),{pid:p.pid,cwd:dir,args:['-n','exact','--',mode],input:'fixture-input',groups:'["a","b"]'});
    assert.ok(err.includes('fixture-error')); assert.equal(code,mode==='exit'?17:null); assert.equal(signal,mode==='signal'?'SIGTERM':null);
  }
});
