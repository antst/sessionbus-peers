#!/usr/bin/env python3
"""Measure the actual Qwen archive and final checkout; no product execution."""
import hashlib
import json
import pathlib
import subprocess
import sys
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[3]
OUTPUT = ROOT / 'docs/designs/qwen-0.5.0/SIZE.json'

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
# All Qwen and common MCP files count, including retained unreferenced Go source.
owned = [r for r in rows if r['path'].startswith(('cmd/qwen-peer/', 'wrappers/qwen/', 'wrappers/mcp/'))]
runtime = [r for r in owned if r['path'].endswith('.go') and not r['path'].endswith('_test.go')]
tests = [r for r in owned if r['path'].endswith('_test.go')]
raw = command('go', 'list', '-deps', '-json', './cmd/qwen-peer')
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
    'installed_runtime_commit': '68e04075155a0072c6121a84a4a4f5bbc5d4e175',
    'scope': 'Whole checkout includes retained source, tests, docs and old installers. SIZE.json has a separate self-size, no impossible self-hash. Dependency source is the non-stdlib files selected by Linux go list, not installed module trees. All stdlib/runtime cost is included in linked binary.',
    'archive': {'bytes': archive.stat().st_size, 'sha256': hashlib.sha256(archive.read_bytes()).hexdigest(),
                'regular': summary(artifacts), 'aliases': aliases},
    'qwen_and_common_mcp_runtime_source': summary(runtime),
    'qwen_and_common_mcp_tests': summary(tests),
    'qwen_plugin_install_assets': summary([r for r in rows if r['path'].startswith('qwen/') or r['path'] in ('scripts/package-product', 'scripts/release/install-product')]),
    'third_party_selected_source': modules,
    'whole_checkout_excluding_this_measurement': summary(rows),
    'whole_checkout_including_this_measurement': {},
}
if len(sys.argv) > 2:
    installed = pathlib.Path(sys.argv[2]).resolve()
    value = json.loads(installed.read_text())
    files = value['installed_files']
    result['real_installed_inventory'] = {
        'record': str(installed),
        'sha256': hashlib.sha256(installed.read_bytes()).hexdigest(),
        'scope': 'Actual permanent Qwen package plus native extension copy at installed68; symlink targets counted once, native extension metadata included. Historical installed68 inventory; final source additionally changes explicit FIFO poll registration. Active skill, lane runtime, scripts and pins stay identical; final archive is measured separately.',
        'entries': len(files),
        'regular_bytes_excluding_symlink_targets': sum(r['bytes'] for r in files if not r['symlink']),
        'members': files,
    }
    result['native_runtime_files'] = value['native_files']
    result['native_version_at_final_capture'] = value['native_version']
    result['runtime_limits'] = {
        'acp_frame_and_output_bytes': 1048576,
        'acp_pending_or_incoming_requests': 256,
        'acp_retained_payload_bytes': 33554432,
        'stage_entries': 256, 'stage_bytes': 131072, 'native_pull_batch': 10,
        'interactive_config_input_and_combined_bytes': 65536,
        'interactive_record_bytes': 8388608,
        'interactive_input_total_byte_ceiling': None,
        'interactive_incoming_bus_work': 32, 'interactive_watched_paths': 8,
        'wrapper_persistence': False,
    }
    result['runtime_cost_notes'] = 'One Go launcher plus one native MCP Go helper for interactive, or one Go Worker plus stateless Go MCP helper for ACP lane. Native Qwen owns its existing Node/process tree. No Qwen-added Go module or Node adapter dependency. FIFO drains native output without a duplicate transcript. Input file grows during live launch; no aggregate byte ceiling or truncation/recovery. No measured peak/idle RSS claim; native process captures are retained in external acceptance evidence.'
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
print(json.dumps({k: {n: v for n, v in result[k].items() if n != 'members'} for k in ['qwen_and_common_mcp_runtime_source', 'qwen_and_common_mcp_tests', 'whole_checkout_including_this_measurement']}, indent=2))
