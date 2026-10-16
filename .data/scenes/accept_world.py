"""Existing-stack transport and independent Writer branches for acceptance probes."""
import copy
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.parse
import uuid

from accept import (ATTACHED, ContractFailure, HarnessError, QINGHE, REPO, ROOT,
                    check, get_json, lookup, material_path, subset)
from goto import parse_feature, split_command, match_has, match_includes, GotoError
from tree import load_yaml_file

if str(REPO/'scripts') not in sys.path:
    sys.path.insert(0, str(REPO/'scripts'))


class World:
    def __init__(self, plans):
        self.plans = plans
        self.prefix = 'accept-' + uuid.uuid4().hex[:16] + '-'
        self.repositories = {name: self.prefix+name for name in
                             ('table-meta', 'sales-semantic', 'ops-shared', 'sales-analysis')}
        self.container = os.environ.get('KC_ACCEPT_CLI_CONTAINER', 'kc-deploy-local-cli-1')
        self.directory = None
        self.base = {}
        self.saved = {}
        self.branch = None
        self.sequence = 0
        self.active = False
        self.origin_before = {}
        self.origin_offsets = {}
        self.origin_changes = {}
        self.events = []
        self.attached = set()
        self.datasets = set()
        self.cleanup_report = {'datasetsRetired': [], 'repositoriesDetached': [], 'search': None}
        self.principals = {}
        schema_ids = set()
        for materials in (QINGHE/'_materials/table-meta', QINGHE/'_materials/sales-semantic',
                          QINGHE/'_materials/acceptance'):
            for file in materials.rglob('*.yaml'):
                for value in re.findall(r'(?m)^object_id:\s*[\"\']?(schema/[^\s\"\']+)', file.read_text()):
                    if not value.startswith('schema/core/'):
                        schema_ids.add(value)
        self.schema_ids = {value: 'schema/'+self.prefix+value[len('schema/'):] for value in schema_ids}
        resource_refs = set()
        for file in (QINGHE/'_materials/table-meta').rglob('*.yaml'):
            resource_refs.update(re.findall(r'descriptorRef:\s*["\']?(resource/[A-Za-z0-9_./-]+)', file.read_text()))
        self.resource_ids = {value: 'resource/'+self.prefix+value.split('/', 1)[1] for value in resource_refs}
        self.descriptor_keys = set()
        for file in (QINGHE/'_materials/acceptance').rglob('*.yaml'):
            self.descriptor_keys.update(re.findall(r'ops/[A-Za-z0-9_./-]+', file.read_text()))
        self.descriptor_ids = {value: 'ops/'+self.prefix+'base/'+value.split('/', 1)[1]
                               for value in self.descriptor_keys}


    def call(self, args, principal='admin'):
        command = ['docker', 'exec', '-i', self.container, '/usr/local/bin/kc',
                   '--server', 'http://kc-server:7380', '--as', principal, *args]
        try:
            result = subprocess.run(command, capture_output=True, text=True, timeout=90)
        except (OSError, subprocess.TimeoutExpired) as exc:
            raise HarnessError(f'existing deployment CLI unavailable: {exc}') from exc
        raw = result.stdout.strip() or result.stderr.strip()
        try:
            value = json.loads(raw)
        except ValueError as exc:
            reason, separator, usage = raw.partition('\n\n')
            if result.returncode == 0 and args in (['help'], ['help', 'consume']):
                value = {'text': raw}
            elif result.returncode and reason.startswith('USAGE_INVALID: ') and separator and usage:
                # The public CLI decorates this one FaultJSON with leaf help.
                value = {'error': {'code': 'USAGE_INVALID', 'message': reason[len('USAGE_INVALID: '):]}}
            else:
                first = raw.splitlines()[0] if raw else ''
                match = re.match(r'^([A-Z][A-Z0-9_]+):', first)
                code = '; code '+match.group(1) if match else ''
                raise HarnessError(f'CLI {self.command_label(args)} returned non-JSON output '
                                   f'(exit {result.returncode}){code}') from exc
        code = value.get('error', {}).get('code') if isinstance(value, dict) else None
        coordinates = {flag[2:]: args[i+1] for i, flag in enumerate(args[:-1])
                       if flag in ('--repo', '--dataset', '--ref', '--commit', '--object', '--aspect')}
        self.events.append({'command': args[:2], 'coordinates': coordinates, 'principal': principal,
                            'exitCode': result.returncode, 'errorCode': code,
                            'valueDigest': hashlib.sha256(raw.encode()).hexdigest()})
        if result.returncode == 0 and args[0] == 'attach':
            self.attached.add(args[args.index('--repo')+1])
        if result.returncode == 0 and args[:2] == ['dataset', 'define']:
            self.datasets.add(args[args.index('--dataset')+1])
        return result.returncode, value

    def require(self, args):
        status, value = self.call(args)
        if status:
            error = value.get('error', value)
            code = error.get('code', 'unknown')
            message = error.get('message', '')
            raise HarnessError(f'prestate {self.command_label(args)} failed: {code}: {message}')
        return value

    @staticmethod
    def command_label(args):
        # Coordinates locate the failed action without logging its input/value.
        label = args[:2]
        for index, arg in enumerate(args[:-1]):
            if arg in ('--repo', '--dataset', '--ref', '--commit', '--file', '--dir', '--name'):
                label += [arg, args[index+1]]
        return ' '.join(label)

    def expand(self, value):
        if isinstance(value, list):
            return [self.expand(item) for item in value]
        if isinstance(value, dict):
            return {key: self.expand(item) for key, item in value.items()}
        if not isinstance(value, str):
            return value
        if value.startswith('$saved.'):
            result, exists = lookup(self.saved, value[len('$saved.'):])
            if not exists:
                raise HarnessError('probe referenced missing saved result: ' + value)
            return result
        variables = {'$prefix': self.prefix, '$dataset': self.dataset,
                     '$branch': self.branch or 'refs/heads/main',
                     '$command': self.prefix+str(self.sequence)}
        for key, replacement in variables.items():
            value = value.replace(key, replacement)
        replacements = dict(self.repositories, **self.principals, **self.schema_ids, **self.resource_ids, **self.descriptor_ids)
        for key, replacement in replacements.items():
            value = re.sub(r'(?<![A-Za-z0-9_-])' + re.escape(key) + r'(?![A-Za-z0-9_-])',
                           lambda _: replacement, value)
        return value

    def prepared_materials(self, source, name):
        target = Path(self.directory.name)/name
        shutil.copytree(source, target)
        for path in target.rglob('*'):
            if path.is_file() and path.suffix in ('.yaml', '.json', '.md', '.sql'):
                path.write_text(self.expand(path.read_text()))
        return target

    def upload(self, local, relative):
        remote = '/tmp/' + self.prefix + '/' + relative
        self.run_docker(['exec', self.container, 'mkdir', '-p', str(Path(remote).parent)])
        self.run_docker(['cp', str(local), self.container+':'+remote])
        return remote

    def run_docker(self, args):
        result = subprocess.run(['docker', *args], capture_output=True, text=True, timeout=30)
        if result.returncode:
            raise HarnessError('existing deployment transport failed: docker ' + args[0])
        return result

    def construct(self):
        """Replay the two existing constructs once, without starting their runtimes."""
        self.directory = tempfile.TemporaryDirectory(prefix=self.prefix)
        self.dataset = self.prefix+'prestate'
        self.active = True
        named = QINGHE.parent
        for node in (named, QINGHE):
            feature = node/'_build/construct.feature'
            material = node/'_materials'
            prepared = self.prepared_materials(material, node.name+'-materials') if material.exists() else None
            remote_material = self.upload(prepared, node.name+'-materials') if prepared else ''
            last = None
            for step in parse_feature(feature.read_text()):
                if step['kind'] == 'skip':
                    continue
                if step['kind'] == 'run':
                    args = [arg.replace('$materials', remote_material) if '$materials' in arg else self.expand(arg)
                            for arg in split_command(step['command'])]
                    if '--command-id' in args:
                        position = args.index('--command-id')+1
                        args[position] = self.prefix+args[position]
                    if args[0] == 'read' and '--repo' not in args:
                        object_id = args[1].split('urn:kc:object:', 1)[-1].split('#', 1)[0]
                        repository = 'sales-semantic' if object_id.startswith(('metric/', 'semantic-model/')) else 'table-meta'
                        args += ['--repo', self.repositories[repository]]
                    last = self.require(args)
                elif step['kind'] in ('has', 'includes'):
                    try:
                        for path, want in step['rows']:
                            matcher = match_has if step['kind'] == 'has' else match_includes
                            matcher(last, path, self.expand(want))
                    except GotoError as exc:
                        raise HarnessError('existing construct assertion: '+str(exc)) from exc
                else:
                    raise HarnessError('unsupported shared construct step: '+step['kind'])
        for name in ('ops-shared', 'sales-analysis'):
            self.require(['create', '--name', self.repositories[name], '--store', 'lakefs'])
            self.require(['attach', '--repo', self.repositories[name]])
        self.branch = 'refs/heads/main'
        for name in ('ops-shared', 'table-meta'):
            for material in sorted((QINGHE/'_materials/acceptance'/name).rglob('*.yaml')):
                self.write_material('$materials/'+str(material.relative_to(QINGHE/'_materials')), name, {})
        for name, repository in self.repositories.items():
            self.base[name] = self.require(['writer', 'head', '--repo', repository])
        # Preserve origin state. Never reset/clear another run's call records.
        for name in ('live', 'untrusted', 'alt'):
            self.origin_before[name] = get_json(self.origin_url(name)+'/control/units')['units']

    def origin_url(self, name):
        return os.environ[{'live': 'KC_ACCEPT_ORIGIN_URL', 'untrusted': 'KC_ACCEPT_UNTRUSTED_ORIGIN_URL',
                           'alt': 'KC_ACCEPT_ALT_ORIGIN_URL'}[name]].rstrip('/')

    def probe(self, spec_id):
        self.saved = {}
        token = hashlib.sha256(spec_id.encode()).hexdigest()[:12]
        self.branch = 'refs/heads/' + self.prefix+token
        self.dataset = self.prefix+token
        self.descriptor_ids = {value: 'ops/'+self.dataset+'/'+value.split('/', 1)[1]
                               for value in self.descriptor_keys}
        self.principals = {name: self.dataset+'-'+name for name in
                           ('alice', 'bob', 'reader', 'auditor', 'carol', 'dave', 'reader-a', 'reader-b')}
        self.origin_offsets = {name: len(get_json(self.origin_url(name)+'/control/calls')['calls'])
                               for name in self.origin_before}
        for repository in self.repositories.values():
            self.require(['writer', 'branch', '--repo', repository, '--name', self.branch])
        try:
            for file in sorted((QINGHE/'_materials/acceptance/ops-shared').rglob('*.yaml')):
                self.write_material('$materials/'+str(file.relative_to(QINGHE/'_materials')), 'ops-shared', {})
            for file in sorted((QINGHE/'_materials/acceptance/table-meta/tables').rglob('*.yaml')):
                if 'valueSource:' in file.read_text():
                    self.write_material('$materials/'+str(file.relative_to(QINGHE/'_materials')), 'table-meta', {})
            for step in self.plans[spec_id]['steps']:
                self.step(step)
        finally:
            self.restore_origins()

    def execute(self, args, principal, expected):
        self.sequence += 1
        args, expected = self.expand(args), self.expand(expected)
        scoped = args[0] in ('read', 'resolve', 'log', 'provenance', 'access', 'invoke') or args[:2] in (
            ['writer', 'put'], ['writer', 'remove'], ['writer', 'head'], ['writer', 'commit'],
            ['schema', 'list'], ['schema', 'describe'], ['binding', 'show'])
        if scoped and '--repo' in args and '--ref' not in args and '--commit' not in args:
            selected_ref = 'refs/heads/main' if self.repositories['ops-shared'] == args[args.index('--repo')+1] else self.branch
            args += ['--ref', selected_ref]
        started = time.monotonic()
        status, value = self.call(args, self.expand(principal))
        elapsed = time.monotonic()-started
        try:
            if elapsed > expected.pop("maxSeconds", float("inf")):
                raise ContractFailure(f'product request exceeded its bounded duration: {elapsed:.3f}s')
            wanted_status = expected.pop('exit', 0)
            if (status != 0) != (wanted_status != 0):
                error = value.get('error', value) if isinstance(value, dict) else {}
                raise ContractFailure(f'exit {status}; expected {wanted_status}; '
                                      f'{error.get("code", "unknown")}: {error.get("message", "")}')
            check(value, expected)
        except ContractFailure as exc:
            raise ContractFailure(self.command_label(args)+': '+str(exc)) from exc
        return value

    def step(self, step):
        if 'kc' in step:
            value = self.execute(step['kc'], step.get('as', 'admin'), dict(step.get('expect', {})))
        elif 'indexDataset' in step:
            value = self.index_dataset(step['indexDataset'])
            check(value, self.expand(step.get('expect', {})))
        elif 'writer' in step:
            value = self.write_material(step['writer'], step.get('repo', 'table-meta'),
                                        dict(step.get('expect', {})), step.get('writerMode', 'put'))
        elif 'trial' in step:
            source = material_path(QINGHE/'_materials', step['trial'])
            self.sequence += 1
            local = Path(self.directory.name)/('trial-'+str(self.sequence)+'.yaml')
            local.write_text(self.expand(source.read_text()))
            remote = self.upload(local, local.name)
            value = self.execute(['plugin', 'declarative-index', 'trial', '--repo', step.get('repo', 'sales-analysis'),
                                  '--ref', self.branch, '--file', remote], step.get('as', 'admin'),
                                 dict(step.get('expect', {})))
        elif 'searchSnapshot' in step:
            from accept_search_resources import snapshot
            repositories = [self.repositories[name] for name in step['searchSnapshot']]
            value = snapshot(os.environ['KC_ACCEPT_OPENSEARCH_URL'], self.prefix, repositories,
                             sorted(self.datasets))
            check(value, self.expand(step.get('expect', {})))
        elif 'origin' in step:
            action = step['origin']
            url = self.origin_url(action.get('name', 'live')) + action['path']
            method = action.get('method', 'GET')
            name = action.get('name', 'live')
            if method in ('PUT', 'DELETE') and action['path'].startswith('/control/units/'):
                address = urllib.parse.unquote(action['path'].split('/control/units/', 1)[1])
                key = (name, address)
                if key not in self.origin_changes:
                    before = get_json(self.origin_url(name)+'/control/units')['units'].get(address)
                    self.origin_changes[key] = [before, None]
                self.origin_changes[key][1] = action.get('body') if method == 'PUT' else None
            value = get_json(url, method, action.get('body'))
            if method == 'GET' and action['path'] == '/control/calls':
                value['calls'] = value['calls'][self.origin_offsets[name]:]
            check(value, self.expand(step.get('expect', {})))
        elif 'assert' in step:
            assertion = self.expand(step['assert'])
            actual = assertion['actual']
            if 'equals' in assertion and actual != assertion['equals']:
                raise ContractFailure('saved observations differ from expected result')
            if 'different' in assertion and actual == assertion['different']:
                raise ContractFailure('expected observations to differ')
            if 'writerDiagnostic' in assertion:
                error = assertion['writerDiagnostic']
                check(actual, {'has': {'code': error['code'],
                                      'reason': error['code']+': '+error['message']}})
            if any(key in assertion for key in ('base64Contains', 'base64Excludes', 'base64EmptyBody')):
                try:
                    decoded = base64.b64decode(actual, validate=True).decode('utf-8')
                except (ValueError, UnicodeDecodeError) as exc:
                    raise ContractFailure('Snapshot maintenance content is not valid base64 UTF-8') from exc
                if assertion.get('base64EmptyBody'):
                    if not decoded.startswith('---\n') or '\n---\n' not in decoded[4:]:
                        raise ContractFailure('Snapshot maintenance bytes lack a complete unit header')
                    if decoded[4:].split('\n---\n', 1)[1].strip():
                        raise ContractFailure('Snapshot external declaration contains a stored body')
                if any(word not in decoded for word in assertion.get('base64Contains', [])):
                    raise ContractFailure('Snapshot maintenance bytes omit required declaration fields')
                if any(word in decoded for word in assertion.get('base64Excludes', [])):
                    raise ContractFailure('Snapshot maintenance bytes contain adopted external values')
            if set(assertion)-{'actual', 'equals', 'different', 'writerDiagnostic', 'base64Contains', 'base64Excludes', 'base64EmptyBody'}:
                raise HarnessError('unknown comparison operator in probe')
            value = actual
        elif 'deployment' in step:
            server = os.environ.get('KC_ACCEPT_SERVER_CONTAINER', 'kc-deploy-local-kc-server-1')
            raw = self.run_docker(['exec', server, 'cat', '/var/lib/kc/deployment.json']).stdout
            try:
                check(json.loads(raw), self.expand(step['deployment']))
            except ContractFailure as exc:
                raise HarnessError('test-deployment variant does not match this specification precondition: '+str(exc)) from exc
            value = {'configurationPrecondition': 'matched'}
        elif 'go' in step:
            names = step['go']['tests']
            if step['go'].get('package') != './internal/arch' or not all(re.fullmatch(r'Test[A-Za-z0-9_]+', name) for name in names):
                raise HarnessError('source probes may only select named architecture assertions')
            pattern = '^('+'|'.join(names)+')$'
            command = [os.environ.get('GO', 'go'), 'test', '-json', '-count=1', '-run', pattern, './internal/arch']
            result = subprocess.run(command, cwd=REPO, capture_output=True, text=True, timeout=120)
            observed = []
            statuses = []
            for line in result.stdout.splitlines():
                try:
                    event = json.loads(line)
                except ValueError:
                    continue
                test, action = event.get('Test', ''), event.get('Action')
                if action in ('pass', 'fail', 'skip') and any(test == name or test.startswith(name+'/') for name in names):
                    statuses.append(action)
                if test in names and action in ('pass', 'fail', 'skip'):
                    observed.append({'name': event['Test'], 'status': event['Action']})
            if 'skip' in statuses or {row['name'] for row in observed} != set(names):
                raise HarnessError('source check did not execute every selected assertion; build errors are not product failures')
            if result.returncode or 'fail' in statuses:
                raise ContractFailure('executed source architecture assertions failed')
            value = {'tests': observed}
            check(value, step['expect'])
        elif 'control' in step:
            control = os.environ.get('KC_ACCEPT_CONTROL_URL')
            if not control:
                raise HarnessError('test-deployment variant/control endpoint is not configured: '+step['control']['action'])
            value = get_json(control.rstrip('/')+'/v1/controls', 'POST', self.expand(step['control']))
            check(value, self.expand(step.get('expect', {})))
        else:
            raise HarnessError('unsupported probe action')
        if step.get('save'):
            self.saved[step['save']] = copy.deepcopy(value)
        for name, selection in step.get('remember', {}).items():
            selected = value
            if 'in' in selection:
                rows, exists = lookup(value, selection['in'])
                matches = [row for row in rows if subset(row, selection['where'])] if exists and isinstance(rows, list) else []
                if len(matches) != 1:
                    raise ContractFailure('cannot select exactly one observed unit for '+name)
                selected = matches[0]
            selected, exists = lookup(selected, selection.get('path', ''))
            if not exists:
                raise ContractFailure('observed result lacks '+selection.get('path', ''))
            self.saved[name] = copy.deepcopy(selected)

    def index_dataset(self, fixture):
        """Publish the authored default declarations and select their scope.

        Each specification starts on its own Writer branch; no result or grant
        is inherited from another probe. The plain control has identical
        source files and omits only the maintenance repository's declarations.
        """
        if set(fixture)-{'name', 'revision', 'declarations', 'metadata'}:
            raise HarnessError('unknown Dataset index fixture option')
        declarations = fixture.get('declarations', True)
        if declarations:
            for member in ('table-name', 'metric-text', 'table-row-count', 'metric-unit'):
                self.write_material('$materials/acceptance/sales-analysis/indexes/sales-search/'+member+'.yaml',
                                    'sales-analysis', {'nonempty': ['result.newCommit']})
        args = ['dataset', 'define', '--dataset', fixture.get('name', '$dataset'),
                '--revision', str(fixture.get('revision', 1))]
        args += ['--source', 'table-meta=$branch@tables@tables',
                 '--source', 'table-meta=$branch@_schemas/table-meta@_schemas',
                 '--source', 'sales-semantic=$branch@metrics@metrics']
        # Both input repositories hold the common Relation Schema. Deliver only
        # the Metric schemas from semantic, keeping one holder per Schema unit.
        for name in ('metric.properties', 'metric.definition'):
            args += ['--file-source', 'sales-semantic=$branch@_schemas/$prefix'+name+
                     '.aspect.yaml@_schemas/sales-semantic/'+name+'.aspect.yaml']
        if declarations:
            args += ['--source', 'sales-analysis=$branch@indexes@indexes']
        if fixture.get('metadata') is not None:
            args += ['--metadata', json.dumps(fixture['metadata'], ensure_ascii=False)]
        return self.execute(args, 'admin', {})

    def write_material(self, value, repository, expected, mode='put'):
        if mode not in ('put', 'commit'):
            raise HarnessError('unsupported Writer material mode: '+str(mode))
        source = material_path(QINGHE/'_materials', value)
        raw = self.expand(source.read_text())
        if not raw.startswith('---\n') or '\n---\n' not in raw[4:]:
            raise HarnessError('object material is not a frontmatter file: '+value)
        header, body = raw[4:].split('\n---\n', 1)
        temp = Path(self.directory.name)/('material-'+str(self.sequence)+'.yaml')
        temp.write_text(raw)
        # The same local YAML parser as the scene tree, without adding a dependency.
        headfile = temp.with_suffix('.header.yaml')
        headfile.write_text(header)
        bodyfile = temp.with_suffix('.body.yaml')
        bodyfile.write_text(body)
        meta = load_yaml_file(headfile)
        # External declarations carry no stored body. The scene-tree parser
        # accepts mappings/lists, so do not turn YAML null into a map parse.
        data = None if body.strip() in ('', 'null', '~') else load_yaml_file(bodyfile)
        self.sequence += 1
        address = 'urn:kc:object:'+meta['object_id']
        if meta.get('aspect_name'):
            address += '#/'+meta['aspect_name']
            if meta.get('member_key'):
                address += '/'+meta['member_key']
        args = ['writer', 'put', address, '--repo', repository,
                '--command-id', self.prefix+str(self.sequence)]
        for field, flag in (('kind', 'kind'), ('schema_ref', 'schema-ref'), ('valueSource', 'value-source')):
            if meta.get(field):
                content = meta[field] if isinstance(meta[field], str) else json.dumps(meta[field])
                args += ['--'+flag, content]
        relative = source.relative_to(QINGHE/'_materials/acceptance')
        hint = meta.get('path_hint', '')
        schema_draft = meta['object_id'].startswith('schema/') and relative.parts[1:2] == ('drafts',)
        if not hint and relative.parts[0] == repository and not schema_draft:
            hint = str(Path(*relative.parts[1:]))
        if hint:
            if repository == 'ops-shared' and self.branch != 'refs/heads/main':
                hint = 'acceptance/'+self.dataset+'/'+hint
            args += ['--path-hint', hint]
        if data is not None:
            args += ['--value', json.dumps(data)]
        else:
            args += ['--value', 'null']
        if mode == 'commit' or set(meta)-{'object_id','aspect_name','member_key','kind','schema_ref','valueSource','path_hint'}:
            # Explicit files and invalid headers pass intact through the public file parser.
            one = temp.parent/('one-'+str(self.sequence))
            one.mkdir()
            shutil.copy2(temp, one/'object.yaml')
            remote = self.upload(one, 'invalid-'+str(self.sequence))
            args = ['writer', 'commit', '--repo', repository, '--command-id', self.prefix+str(self.sequence), '--dir', remote]
        return self.execute(args, 'admin', expected)

    def restore_origins(self):
        for (name, address), (before, last) in list(self.origin_changes.items()):
            current = get_json(self.origin_url(name)+'/control/units')['units'].get(address)
            if current != last:
                raise HarnessError('external-source fixture changed concurrently; refusing to overwrite '+address)
            url = self.origin_url(name)+'/control/units/'+urllib.parse.quote(address, safe='')
            if before is None:
                get_json(url, 'DELETE')
            else:
                get_json(url, 'PUT', before)
            del self.origin_changes[(name, address)]

    def close(self):
        errors = []
        if self.active:
            try:
                self.restore_origins()
            except Exception as exc:
                errors.append('source fixture cleanup: '+str(exc))
            for dataset in sorted(self.datasets):
                if not dataset.startswith(self.prefix):
                    errors.append('refused to retire a Dataset outside this invocation')
                    continue
                try:
                    status, value = self.call(['dataset', 'retire', '--dataset', dataset])
                    if status or value.get('retired') is not True:
                        raise HarnessError('owned Dataset retirement was not acknowledged')
                    self.cleanup_report['datasetsRetired'].append(dataset)
                except Exception as exc:
                    errors.append(str(exc))
            for repository in sorted(self.attached):
                if not repository.startswith(self.prefix):
                    errors.append('refused to detach a Repository outside this invocation')
                    continue
                try:
                    status, _ = self.call(['detach', '--repo', repository])
                    if status:
                        raise HarnessError('owned Repository detach failed')
                    self.cleanup_report['repositoriesDetached'].append(repository)
                except Exception as exc:
                    errors.append(str(exc))
            try:
                from accept_search_resources import cleanup
                self.cleanup_report['search'] = cleanup(os.environ['KC_ACCEPT_OPENSEARCH_URL'], self.prefix,
                                                       list(self.repositories.values()), sorted(self.datasets))
            except Exception as exc:
                errors.append('owned search cleanup: '+str(exc))
            try:
                self.run_docker(['exec', self.container, 'rm', '-rf', '/tmp/'+self.prefix])
            except Exception as exc:
                errors.append('owned CLI material cleanup: '+str(exc))
        if self.directory:
            self.directory.cleanup()
        if errors:
            raise HarnessError('; '.join(errors))
