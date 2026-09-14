#!/usr/bin/env python3
"""Measure checkout, selected Go sources, archive and permanent files; no native launch.

Usage: measure.py ARCHIVE INSTALLED_INVENTORY [ARTIFACT_SOURCE_COMMIT]
Run from the checkout whose files are being measured. Categories overlap.
"""
import hashlib
import json
import os
import pathlib
import subprocess
import sys
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[3]
OUTPUT = ROOT / 'docs/designs/opencode-0.5.0/SIZE.json'


def command(*args):
    return subprocess.check_output(args, cwd=ROOT, text=True,
                                   env={**os.environ, 'GOWORK': 'off', 'GOOS': 'linux', 'GOARCH': 'amd64', 'CGO_ENABLED': '0'})


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def member(path, label=None):
    raw = path.read_bytes()
    return {'path': label or str(path.relative_to(ROOT)), 'bytes': len(raw),
            'lines': len(raw.splitlines()), 'sha256': digest(raw)}


def summary(rows):
    return {'files': len(rows), 'bytes': sum(r['bytes'] for r in rows),
            'lines': sum(r.get('lines', 0) for r in rows), 'members': rows}


archive = pathlib.Path(sys.argv[1]).resolve()
installed = pathlib.Path(sys.argv[2]).resolve()
artifacts, links, directories = [], [], []
with tarfile.open(archive) as package:
    for item in package:
        if item.isfile():
            raw = package.extractfile(item).read()
            artifacts.append({'path': item.name, 'bytes': len(raw),
                              'sha256': digest(raw), 'mode': oct(item.mode)})
        elif item.issym() or item.islnk():
            links.append({'path': item.name, 'target': item.linkname, 'mode': oct(item.mode)})
        elif item.isdir():
            directories.append({'path': item.name, 'mode': oct(item.mode)})
        else:
            raise ValueError('unexpected archive member type: ' + item.name)
paths = sorted(set(command('git', 'ls-files', '--cached', '--others', '--exclude-standard').splitlines()))
rows = [member(ROOT / p) for p in paths if (ROOT / p).is_file() and ROOT / p != OUTPUT]
shipped_js_paths = {'opencode/' + r['path'].removeprefix('plugin/') for r in artifacts
                    if r['path'].startswith('plugin/') and '/node_modules/' not in r['path'] and r['path'].endswith('.mjs')}
owned = [r for r in rows if r['path'].startswith(('cmd/opencode-peer/', 'wrappers/opencode/', 'wrappers/mcp/'))]
go_runtime = [r for r in owned if r['path'].endswith('.go') and not r['path'].endswith('_test.go')]
go_tests = [r for r in owned if r['path'].endswith('_test.go')]
# main-module selected files include shared Worker/host dependencies beyond the
# product directories. This overlaps the directory inventory above.
raw = command('go', 'list', '-deps', '-json', './cmd/opencode-peer')
decoder = json.JSONDecoder()
packages = []
while raw.strip():
    raw = raw.lstrip()
    value, offset = decoder.raw_decode(raw)
    packages.append(value)
    raw = raw[offset:]
modules, main_selected = {}, {}
for package in packages:
    module = package.get('Module', {})
    if not module:
        continue
    record = modules.setdefault(module['Path'], {'version': module.get('Version'), 'members': {}}) if not module.get('Main') else None
    directory = pathlib.Path(package['Dir'])
    for name in package.get('GoFiles', []) + package.get('SFiles', []) + package.get('EmbedFiles', []):
        path = directory / name
        if not path.is_file():
            continue
        label = str(path.relative_to(ROOT if module.get('Main') else pathlib.Path(module['Dir'])))
        (main_selected if module.get('Main') else record['members'])[label] = member(path, label)
for value in modules.values():
    value.update(summary(sorted(value.pop('members').values(), key=lambda r: r['path'])))
observed = json.loads(installed.read_text())
files = observed['installed_files']
# The existing inventory records physical permanent members; reject aliases in
# this set to prevent counting one target twice. Front-door aliases are separate.
assert all(r['exists'] and not r['symlink'] for r in files)
prefix = '/home/antst/.local/libexec/sessionbus/opencode/'
actual = {r['path'].removeprefix(prefix): r for r in files}
expected = {r['path']: r for r in artifacts if r['path'] not in ('install', 'ROLE')}
assert actual.keys() == expected.keys(), 'permanent inventory differs from archive members'
for name, row in actual.items():
    assert row['bytes'] == expected[name]['bytes'] and row['sha256'] == expected[name]['sha256'], name
# install -m fixes executable mode; cp into existing top-level files can retain
# their prior modes. Record both instead of asserting nonexistent mode equality.
assert int(actual['opencode-peer']['mode'], 8) == 0o755
mode_differences = [{'path': n, 'archive': expected[n]['mode'], 'installed': r['mode']}
                    for n, r in actual.items() if int(r['mode'], 8) != int(expected[n]['mode'], 8)]
native = [r for r in observed['files'] if r.get('exists') and (r['path'].endswith('/bin/opencode') or r['path'].endswith('/opencode-ai/package.json'))]
assert len({r['resolved'] for r in native}) == len(native)
result = {
    'measured_source_commit': command('git', 'rev-parse', 'HEAD').strip(),
    'artifact_source_commit': sys.argv[3] if len(sys.argv) > 3 else command('git', 'rev-parse', 'HEAD').strip(),
    'native_acceptance_runtime_commit': '502d2b1f0740576cc6dd493460fa98c22edc6b4c',
    'measurement_target': json.loads(command('go', 'env', '-json', 'GOVERSION', 'GOOS', 'GOARCH', 'CGO_ENABLED')),
    'scope': 'Whole checkout includes retained source, tests, docs and historical measurements. SIZE.json has separate self-size, no self-hash. Source categories overlap; do not sum. Dependency files are selected by Linux amd64 go list, not entire module caches. Linked Go binary includes runtime/stdlib. Native inventory counts resolved targets once and is not a full native package or RSS measurement.',
    'archive': {'bytes': archive.stat().st_size, 'sha256': digest(archive.read_bytes()),
                'regular': summary(artifacts), 'aliases': links, 'directories': directories},
    'opencode_and_common_mcp_go_runtime_source': summary(go_runtime),
    'opencode_and_common_mcp_go_tests': summary(go_tests),
    'selected_main_module_go_source': summary(sorted(main_selected.values(), key=lambda r: r['path'])),
    'native_js_runtime_source': summary([r for r in rows if r['path'] in shipped_js_paths]),
    'native_js_tests': summary([r for r in rows if r['path'].startswith('opencode/') and r['path'].endswith('.mjs') and r['path'] not in shipped_js_paths]),
    'plugin_declaration_skill_manifest_and_install_assets': summary([r for r in rows if (r['path'].startswith(('opencode/', 'internal/cmd/gen-opencode-tool/')) and not r['path'].endswith('.mjs')) or r['path'] in ('scripts/package-product', 'scripts/release/install-product', 'scripts/install-opencode.sh')]),
    'third_party_selected_go_source': modules,
    'shipped_js_kit': summary([r for r in artifacts if r['path'].startswith('plugin/node_modules/@sessionbus/kit/')]),
    'whole_checkout_excluding_this_measurement': summary(rows),
    'whole_checkout_including_this_measurement': {},
    'real_installed_inventory': {'record': str(installed), 'sha256': digest(installed.read_bytes()),
                                'archive_names_sizes_hashes_equal': True, 'mode_differences': mode_differences, 'entries': len(files),
                                'regular_bytes': sum(r['bytes'] for r in files), 'members': files,
                                'front_door_aliases_not_counted_twice': [r for r in observed['files'] if r['path'].endswith('/bin/opencode-peer')]},
    'existing_native_runtime': {'package': observed['native_package'], 'unique_targets': len(native),
                                'bytes': sum(r['bytes'] for r in native), 'members': native},
    'runtime_limits': {'interactive_owners_including_retiring': 128, 'pending_identity': 16, 'interactive_http': 256,
                       'per_owner_fifo_messages': 64, 'per_owner_fifo_bytes': 1048576, 'all_owner_fifo_bytes': 16777216,
                       'peer_shared_tool_delivery_tasks': 256, 'bridge_connections': 8, 'bridge_ingress_bytes': 2097152,
                       'bridge_response_bytes': 8388608, 'bridge_work': 256, 'bridge_retained_bytes': 33554432,
                       'lane_http_slots_including_one_control': 8, 'lane_header_bytes': 65536,
                       'lane_request_and_output_bytes': 1048576, 'lane_response_bytes': 8388608,
                       'history_page_messages': 64, 'history_messages': 4096, 'history_bytes': 16777216,
                       'wrapper_history_or_restart_replay': False},
    'runtime_cost_notes': 'Go launcher or Worker plus its existing native child; native in-process JS hooks use existing Bun, supplied SDK client and host Solid. One native loopback HTTP listener plus bounded resident Unix bridge, no extra broker process or target Node/Bun installer. Only JS runtime dependency is pinned Sessionbus kit; Go dependency source listed above. Other configured native MCP descendants are not Sessionbus additions. Native SDK response allocation lies outside wrapper budgets. No measured idle/peak RSS. SIGKILL can leave native and private directory; actual PTY row had native disappearance and separate operator directory removal.',
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
print(json.dumps({k: {n: v for n, v in result[k].items() if n != 'members'} for k in ['opencode_and_common_mcp_go_runtime_source', 'native_js_runtime_source', 'whole_checkout_including_this_measurement']}, indent=2))
