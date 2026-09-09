// SPDX-License-Identifier: MIT
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, writeFileSync, realpathSync, existsSync, rmSync, cpSync, readdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname, delimiter } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync, spawn } from 'node:child_process';
import { once } from 'node:events';

const root = fileURLToPath(new URL('.', import.meta.url));
const json = path => JSON.parse(readFileSync(path, 'utf8'));
const qualified = 'mcp__plugin_sessionbus_sessionbus__sessionbus';

for (const route of ['packed','folder']) test(`${route} npm install resolves only the active plugin from a root with spaces`, async t => {
  const dir=mkdtempSync(join(tmpdir(),'claude package ')); t.after(()=>rmSync(dir,{recursive:true,force:true}));
  const npm=(args,cwd=root)=>execFileSync('npm',args,{cwd,encoding:'utf8',stdio:['ignore','pipe','pipe']});
  assert.deepEqual(readdirSync(join(root,'skills')),['sessionbus']);
  const [pack]=JSON.parse(npm(['pack','--json','--pack-destination',dir]));
  const paths=pack.files.map(f=>f.path);
  for(const path of ['main.mjs','mcp.mjs','owner.mjs','delivery.mjs','tools.mjs','.mcp.json',
    '.claude-plugin/plugin.json','hooks/hooks.json','skills/sessionbus/SKILL.md','commands/doctor.md','README.md']) assert.ok(paths.includes(path),path);
  assert.ok(!paths.some(p=>p.endsWith('.test.mjs')||p.startsWith('node_modules/')));
  assert.deepEqual(paths.filter(p=>p.startsWith('skills/')),['skills/sessionbus/SKILL.md']);
  const source=join(dir,'source folder');
  if(route==='folder') {
    cpSync(root,source,{recursive:true,filter:path=>!path.includes('node_modules')});
    npm(['ci','--prefix',source,'--offline','--ignore-scripts','--no-audit','--no-fund']);
  }
  const prefix=join(dir,'installed prefix');
  npm(['install','--global','--prefix',prefix,route==='packed'?join(dir,pack.filename):source,'--offline','--ignore-scripts','--no-audit','--no-fund']);
  const bin=join(prefix,'bin/claude-peer'); const installed=dirname(realpathSync(bin));
  assert.equal(installed,route==='packed'?join(prefix,'lib/node_modules/@sessionbus/claude'):source);
  assert.deepEqual(readdirSync(join(installed,'skills')),['sessionbus']);
  const manifest=json(join(installed,'package.json'));
  assert.deepEqual(manifest.bin,{'claude-peer':'main.mjs'});
  assert.equal(existsSync(join(prefix,'bin/sessionbus-claude-mcp')),false);
  assert.equal(manifest.dependencies['@sessionbus/kit'],'https://pkg.pr.new/@sessionbus/kit@5f93fbb');
  assert.equal(json(join(installed,'.claude-plugin/plugin.json')).version,manifest.version);
  const server=json(join(installed,'.mcp.json')).mcpServers.sessionbus;
  assert.deepEqual(server,{command:'node',args:['${CLAUDE_PLUGIN_ROOT}/mcp.mjs']});
  const hooks=json(join(installed,'hooks/hooks.json')).hooks;
  assert.deepEqual(Object.keys(hooks).sort(),['SessionEnd','Stop','UserPromptSubmit']);
  for(const entries of Object.values(hooks)) for(const group of entries) for(const hook of group.hooks) {
    assert.equal(hook.type,'mcp_tool'); assert.equal(hook.server,'plugin:sessionbus:sessionbus');
    assert.equal(hook.tool,'native_identity_event');
  }
  // Execute only a tiny local fixture, never the real Claude product.
  const fixture=join(dir,'claude');
  writeFileSync(fixture,`#!${process.execPath}\nprocess.stdout.write(JSON.stringify(process.argv.slice(2)));\n`,{mode:0o755});
  const env={...process.env,PATH:dir+delimiter+dirname(process.execPath),SESSIONBUS_LAUNCH_TOKEN:''};
  const argv=['--','bare prompt','--disallowedTools',qualified,'-g','one'];
  const out=execFileSync(bin,argv,{env,encoding:'utf8'});
  assert.deepEqual(JSON.parse(out),['--allowedTools',qualified,'--plugin-dir',installed,...argv.slice(0,-2)]);
  assert.throws(()=>execFileSync(bin,[],{env:{...env,SESSIONBUS_LAUNCH_TOKEN:'held'},stdio:'pipe'}),e=>e.status===1 && e.stderr.toString().includes('lane mode is unavailable'));
  // Native-root substitution resolves the real packaged stdio entry. No report/dial is sent.
  const entry=server.args[0].replace('${CLAUDE_PLUGIN_ROOT}',installed);
  assert.throws(()=>execFileSync(process.execPath,[entry],{env:{...env,SESSIONBUS_LAUNCH_TOKEN:'held'},stdio:'pipe'}),e=>e.status===1 && e.stderr.toString().includes('lane mode is unavailable'));
  const child=spawn(process.execPath,[entry],{env:{...process.env,SESSIONBUS_LAUNCH_TOKEN:''},stdio:'pipe'});
  let stdout='',stderr='';child.stdout.on('data',b=>stdout+=b);child.stderr.on('data',b=>stderr+=b);
  const closed=once(child,'close');child.stdin.end(JSON.stringify({jsonrpc:'2.0',id:1,method:'tools/list'})+'\n');
  const [code,signal]=await closed;assert.equal(code,0);assert.equal(signal,null);assert.equal(stderr,'');
  const frame=JSON.parse(stdout.trim());assert.equal(frame.id,1);assert.deepEqual(frame.result.tools.map(t=>t.name),['sessionbus']);
  assert.notEqual(frame.result.tools[0].annotations?.readOnlyHint,true);
  npm(['uninstall','--global','--prefix',prefix,'@sessionbus/claude','--ignore-scripts','--no-audit','--no-fund']);
  assert.equal(existsSync(bin),false);assert.equal(existsSync(join(prefix,'lib/node_modules/@sessionbus/claude')),false);
  assert.equal(existsSync(installed),route==='folder'); // npm keeps linked source on uninstall
});
