#!/usr/bin/env python3
"""Measure the actual Codex archive and final checkout; no product execution."""
import hashlib
import json
import pathlib
import subprocess
import sys
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[3]
OUTPUT = ROOT / 'docs/designs/codex-0.5.0/SIZE.json'

def command(*args):
    return subprocess.check_output(args, cwd=ROOT, text=True)

def member(path, label=None):
    raw = path.read_bytes()
    return {'path': label or str(path.relative_to(ROOT)), 'bytes': len(raw),
            'lines': len(raw.splitlines()), 'sha256': hashlib.sha256(raw).hexdigest()}

def summary(rows):
    return {'files': len(rows), 'bytes': sum(r['bytes'] for r in rows),
            'lines': sum(r.get('lines', 0) for r in rows), 'members': rows}

archive = pathlib.Path(sys.argv[1]).resolve()
artifacts = []
aliases = []
with tarfile.open(archive) as package:
    for item in package:
        if item.isfile():
            raw = package.extractfile(item).read()
            artifacts.append({'path': item.name, 'bytes': len(raw),
                              'sha256': hashlib.sha256(raw).hexdigest()})
        elif item.issym():
            aliases.append({'path': item.name, 'target': item.linkname})
paths = sorted(set(command('git', 'ls-files', '--cached', '--others', '--exclude-standard').splitlines()))
rows = [member(ROOT / p) for p in paths if (ROOT / p).is_file() and ROOT / p != OUTPUT]
# All Codex and common MCP files count, including retained unreferenced Go source.
owned = [r for r in rows if r['path'].startswith(('cmd/codex-peer/', 'wrappers/codex/', 'wrappers/mcp/'))]
runtime = [r for r in owned if r['path'].endswith('.go') and not r['path'].endswith('_test.go')]
tests = [r for r in owned if r['path'].endswith('_test.go')]
raw = command('go', 'list', '-deps', '-json', './cmd/codex-peer')
decoder = json.JSONDecoder()
packages = []
while raw.strip():
    raw = raw.lstrip()
    value, offset = decoder.raw_decode(raw)
    packages.append(value)
    raw = raw[offset:]
modules = {}
for package in packages:
    module = package.get('Module', {})
    if not module or module.get('Main'):
        continue
    key = module['Path']
    record = modules.setdefault(key, {'version': module.get('Version'), 'members': []})
    directory = pathlib.Path(package['Dir'])
    for name in package.get('GoFiles', []) + package.get('SFiles', []) + package.get('EmbedFiles', []):
        p = directory / name
        if p.is_file():
            label = str(p.relative_to(module['Dir']))
            if not any(r['path'] == label for r in record['members']):
                record['members'].append(member(p, label))
for value in modules.values():
    value.update(summary(sorted(value.pop('members'), key=lambda r: r['path'])))
result = {
    'measured_source_commit': command('git', 'rev-parse', 'HEAD').strip(),
    'scope': 'Whole checkout includes retained source, tests, docs and old installers. SIZE.json has a separate self-size, no impossible self-hash. Dependency source is the non-stdlib files selected by Linux go list, not installed module trees. All stdlib/runtime cost is included in linked binary.',
    'archive': {'bytes': archive.stat().st_size, 'sha256': hashlib.sha256(archive.read_bytes()).hexdigest(),
                'regular': summary(artifacts), 'aliases': aliases},
    'codex_and_common_mcp_runtime_source': summary(runtime),
    'codex_and_common_mcp_tests': summary(tests),
    'codex_plugin_install_assets': summary([r for r in rows if r['path'].startswith('codex/') or r['path'] == 'scripts/package-codex']),
    'third_party_selected_source': modules,
    'whole_checkout_excluding_this_measurement': summary(rows),
    'whole_checkout_including_this_measurement': {},
}
for _ in range(20):
    text = json.dumps(result, indent=2) + '\n'
    total = {'files': len(rows) + 1, 'bytes': sum(r['bytes'] for r in rows) + len(text.encode()),
             'lines': sum(r['lines'] for r in rows) + len(text.splitlines()), 'measurement_bytes': len(text.encode())}
    if result['whole_checkout_including_this_measurement'] == total:
        OUTPUT.write_text(text)
        break
    result['whole_checkout_including_this_measurement'] = total
else:
    raise RuntimeError('self-size did not stabilize')
print(json.dumps({k: {n: v for n, v in result[k].items() if n != 'members'} for k in ['codex_and_common_mcp_runtime_source', 'codex_and_common_mcp_tests', 'whole_checkout_including_this_measurement']}, indent=2))
