#!/usr/bin/env python3
"""Read-only b71 source/artifact measurement; writes SIZE.json only; SIZE.md is the accompanying reviewed draft.

Usage: python3 measure.py BUILD.json ARCHIVE_DIR INSTALLED_INVENTORY.json
Run in a worktree whose tracked source equals BUILD.source_commit. Requires the
existing Go toolchain/module cache; GOPROXY=off prevents dependency downloads.
No package build, target process, native CLI, or host connection is performed.
"""
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent
PRODUCTS = ('opencode', 'kilo')
ENV = {**os.environ, 'GOWORK': 'off', 'GOOS': 'linux', 'GOARCH': 'amd64',
       'CGO_ENABLED': '0', 'GOPROXY': 'off', 'GOTOOLCHAIN': 'local'}


def run(*args, binary=False):
    return subprocess.check_output(args, cwd=ROOT, env=ENV, text=not binary)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def row(name, raw, lines=True):
    result = {'path': name, 'bytes': len(raw), 'sha256': sha(raw)}
    if lines:
        result['lines'] = len(raw.splitlines())
    return result


def total(rows):
    rows = sorted(rows, key=lambda x: x['path'])
    return {'files': len(rows), 'bytes': sum(x['bytes'] for x in rows),
            'lines': sum(x.get('lines', 0) for x in rows), 'members': rows}


def binding(path):
    return row(str(path), path.read_bytes(), False)


def decode_stream(raw):
    decoder = json.JSONDecoder()
    while raw.strip():
        raw = raw.lstrip()
        obj, offset = decoder.raw_decode(raw)
        yield obj
        raw = raw[offset:]


def source_group(path):
    if path.startswith('wrappers/opencodefamily/plugin/'):
        if path.endswith('.mjs'):
            return 'family_js_tests_and_fixtures' if ('test.mjs' in path or 'fixture.mjs' in path) else 'family_js_runtime'
        return 'family_plugin_assets'
    for prefix, group in [('wrappers/opencodefamily/', 'family_go'),
                          ('wrappers/mcp/', 'shared_mcp_go')]:
        if path.startswith(prefix) and path.endswith('.go'):
            return group + ('_tests' if path.endswith('_test.go') else '_runtime')
    for product in PRODUCTS:
        if path.startswith((f'wrappers/{product}/', f'cmd/{product}-peer/')) and path.endswith('.go'):
            return product + ('_go_tests' if path.endswith('_test.go') else '_go_runtime')
        if path.startswith(product + '/'):
            return product + '_manifests_and_docs'
    if path.startswith(('internal/pluginstage/', 'internal/cmd/stage-native-plugin/', 'internal/cmd/gen-opencode-tool/')) or path in (
            'scripts/package-product', 'scripts/release/install-product', 'scripts/install-opencode.sh'):
        return 'shared_build_sources_and_tests'
    return 'other_checkout_source_tests_docs_assets'


def measure(build_path, archive_dir, installed_path):
    build = json.loads(build_path.read_text())
    pin = build['source_commit']
    # Added measurement documents are excluded; runtime/source discrepancies fail.
    changed = run('git', 'diff', '--name-only', pin, '--').splitlines()
    assert all(p.startswith('docs/designs/kilo-0.5.0/') for p in changed), changed
    groups, source = {}, {}
    with tarfile.open(fileobj=io.BytesIO(run('git', 'archive', pin, binary=True))) as archive:
        for item in archive:
            if item.isfile():
                raw = archive.extractfile(item).read()
                source[item.name] = row(item.name, raw)
                groups.setdefault(source_group(item.name), []).append(source[item.name])
    recorded = {x['path']: x for x in build['source_files']}
    assert recorded.keys() == source.keys(), 'BUILD source member set differs'
    assert all(source[p]['sha256'] == recorded[p]['sha256'] for p in source), 'BUILD source bytes differ'
    dependencies, selected = {}, {}
    for product in PRODUCTS:
        selected[product] = set()
        for package in decode_stream(run('go', 'list', '-deps', '-json', './cmd/' + product + '-peer')):
            module = package.get('Module')
            if not module:  # Standard library is already included in linked binary size.
                continue
            module_root = ROOT if module.get('Main') else Path(module['Dir'])
            for name in sorted(set(sum((package.get(k, []) for k in ('GoFiles', 'CgoFiles', 'CFiles', 'HFiles', 'SFiles', 'SysoFiles', 'EmbedFiles')), []))):
                path = Path(package['Dir']) / name
                label = str(path.relative_to(module_root))
                key = module['Path'] + ':' + label
                selected[product].add(key)
                if key not in dependencies:
                    item = row(key, path.read_bytes())
                    item.update(module=module['Path'], version=module.get('Version'), main=bool(module.get('Main')))
                    if item['main']:
                        assert label in source and item['sha256'] == source[label]['sha256'], label
                    dependencies[key] = item
    selection = {}
    common = selected['opencode'] & selected['kilo']
    for name, keys in [('common', common), *[(p + '_only', selected[p] - common) for p in PRODUCTS]]:
        selection[name] = {kind: total([dependencies[k] for k in keys if dependencies[k]['main'] == is_main])
                           for kind, is_main in [('main_module', True), ('external_modules', False)]}
    observed = json.loads(installed_path.read_text())
    artifacts = {}
    for product in PRODUCTS:
        path = archive_dir / (product + '-peer-linux-amd64.tar.gz')
        assert sha(path.read_bytes()) == build['archives'][product]['sha256'], path
        regular, aliases, payload = [], [], {}
        with tarfile.open(path) as archive:
            for item in archive:
                if item.isfile():
                    raw = archive.extractfile(item).read()
                    payload[item.name] = raw
                    regular.append({**row(item.name, raw, False), 'mode': oct(item.mode)})
                elif item.issym() or item.islnk():
                    aliases.append({'path': item.name, 'target': item.linkname})
                elif not item.isdir():
                    raise ValueError('unexpected archive member ' + item.name)
        expected_build = {x['name']: x for x in build['archives'][product]['members'] if x['type'] == '0'}
        assert set(payload) == set(expected_build)
        assert all(sha(raw) == expected_build[name]['sha256'] for name, raw in payload.items())
        prefix = observed['home'] + '/.local/libexec/sessionbus/' + product + '/'
        installed = [x for x in observed['installed_files'] if x['path'].startswith(prefix)]
        actual = {x['path'][len(prefix):]: x for x in installed if not x['symlink']}
        expected = {x['path']: x for x in regular if x['path'] not in ('install', 'ROLE')}
        assert actual.keys() == expected.keys(), (product, actual.keys() ^ expected.keys())
        assert all(x['exists'] and x['bytes'] == expected[n]['bytes'] and x['sha256'] == expected[n]['sha256'] for n, x in actual.items())
        native_js = [x for x in regular if x['path'].startswith('plugin/') and x['path'].endswith('.mjs') and '/node_modules/' not in x['path']]
        assert {x['path'].removeprefix('plugin/') for x in native_js} == {x['path'].removeprefix('wrappers/opencodefamily/plugin/') for x in groups['family_js_runtime']}
        for item in native_js:
            canonical = 'wrappers/opencodefamily/' + item['path']
            assert canonical in source and item['sha256'] == source[canonical]['sha256']
        binary = product + '-peer'
        with tempfile.TemporaryDirectory(prefix='family-size-') as temporary:
            binary_path = Path(temporary) / binary
            binary_path.write_bytes(payload[binary])
            info_lines = run('go', 'version', '-m', str(binary_path)).splitlines()
            binary_go_version = info_lines[0].split(': ', 1)[1]
            build_info = info_lines[1:]
        artifacts[product] = {
            'archive': binding(path), 'regular': total(regular), 'archive_aliases': aliases,
            'binary': next(x for x in regular if x['path'] == binary), 'binary_go_version': binary_go_version,
            'go_build_info': build_info,
            'plugin_manifest': json.loads(payload['plugin/package.json']),
            'plugin_lock': json.loads(payload['plugin/package-lock.json']),
            'canonical_js_copies': total(native_js),
            'kit': total([x for x in regular if x['path'].startswith('plugin/node_modules/@sessionbus/kit/')]),
            'rendered_skill': row('plugin/skills/sessionbus/SKILL.md', payload['plugin/skills/sessionbus/SKILL.md']),
            'installed_regular': total([{**x, 'path': n} for n, x in actual.items()]),
            'installed_archive_bytes_equal': True,
            'aliases_not_added_to_regular_bytes': [x for x in installed if x['symlink']] + [observed['executables'][binary]],
        }
    native_records = [observed['executables'][p] for p in PRODUCTS]
    native_records += [x for x in observed['native_caches'] if x.get('exists')]
    native_records += [x for x in observed['packages'] if x.get('metadata', {}).get('name') in ('opencode-ai', '@kilocode/cli')]
    unique = {x['resolved']: x for x in native_records}
    return {
        'source_commit': pin, 'bindings': {'build': binding(build_path), 'installed_inventory': binding(installed_path)},
        'measurement_target': json.loads(run('go', 'env', '-json', 'GOVERSION', 'GOOS', 'GOARCH', 'CGO_ENABLED')),
        'artifact_go_version': build['go_version'],
        'source_groups_disjoint': {k: total(v) for k, v in sorted(groups.items())},
        'whole_pinned_checkout': total(source.values()),
        'selected_go_dependencies_disjoint_between_common_and_product_only': selection,
        'archives_and_installations': artifacts,
        'installed_two_product_regular_bytes': sum(x['installed_regular']['bytes'] for x in artifacts.values()),
        'observed_existing_native_files_only': total(unique.values()),
        'scope': ['Source groups partition the pinned checkout; canonical shared files count once. Whole checkout is an overlapping total.',
                  'Go dependency selections overlap source groups. Common/only sets are disjoint; external selected files exclude unselected module cache files and stdlib.',
                  'Archive subcategories overlap regular total. Physical copies across products count twice; aliases add no target bytes. Lines are physical splitlines, not executable LOC.',
                  'Measurement documents are not in the pinned source total. Build and installed snapshots are historical exact inputs, not a current host query.',
                  'Linked Go binaries include Go runtime and standard library. Native files are a partial existing prerequisite inventory, not incremental wrapper cost or a full native package measurement.',
                  'No RSS, allocation peak, whole native package, daemon storage, filesystem block allocation, or running process resource usage measured.']}


if __name__ == '__main__':
    if len(sys.argv) != 4:
        raise SystemExit(__doc__)
    result = measure(*(Path(p).resolve() for p in sys.argv[1:]))
    (OUT / 'SIZE.json').write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps({k: {'files': v['files'], 'bytes': v['bytes'], 'lines': v['lines']} for k, v in result['source_groups_disjoint'].items()}, indent=2))
