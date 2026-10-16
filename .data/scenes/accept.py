#!/usr/bin/env python3
"""Run explicit acceptance probes against an existing local deployment only."""
from __future__ import annotations

import argparse
import datetime
import importlib.util
import json
import os
import signal
from pathlib import Path
import sys
import urllib.error
import urllib.request
import uuid

if __name__ == "__main__":
    sys.modules["accept"] = sys.modules[__name__]

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))
from tree import load_yaml_file  # noqa: E402

QINGHE = ROOT/'service-initialized/named-repositories-created/qinghe-object-published'
ATTACHED = ROOT/'service-initialized/source-repositories-configured/repository-attached'
PROBES = QINGHE/'_probes/acceptance/probes.json'


class ContractFailure(Exception):
    """An executed product action violated an explicit observable assertion."""


class RunInterrupted(BaseException):
    def __init__(self, signum, *, started=True):
        self.signum = signum
        self.started = started
        super().__init__(f'acceptance interrupted by signal {signum}')


class HarnessError(Exception):
    """A fixture, environment, unknown command shape or executor is unavailable."""


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def lookup(value, path):
    for key in path.split('.') if path else []:
        if isinstance(value, list) and key.isdecimal():
            if int(key) >= len(value):
                return None, False
            value = value[int(key)]
        elif isinstance(value, dict) and key in value:
            value = value[key]
        else:
            return None, False
    return value, True


def subset(value, wanted):
    if isinstance(wanted, dict):
        return isinstance(value, dict) and all(k in value and subset(value[k], v) for k, v in wanted.items())
    return value == wanted


def check(value, expected):
    if set(expected)-{'has','nonempty','absent','includes','count','find','contains','excludes','forEach','unitAddresses'}:
        raise HarnessError('unknown observable assertion operator')
    for path, want in expected.get('has', {}).items():
        got, exists = lookup(value, path)
        if not exists or got != want:
            raise ContractFailure(f'{path}: got {got!r} (present={exists}); want {want!r}')
    for path in expected.get('nonempty', []):
        got, exists = lookup(value, path)
        if not exists or got in (None, '', [], {}):
            raise ContractFailure(f'{path}: expected a nonempty value')
    for path in expected.get('absent', []):
        if lookup(value, path)[1]:
            raise ContractFailure(f'{path}: expected absent')
    for path, wants in expected.get('includes', {}).items():
        got, exists = lookup(value, path)
        if not exists or not isinstance(got, list):
            raise ContractFailure(f'{path}: expected array')
        for want in wants:
            if not any(subset(item, want) for item in got):
                raise ContractFailure(f'{path}: missing {want!r}')
    for path, want in expected.get('count', {}).items():
        got, exists = lookup(value, path)
        if not exists or not isinstance(got, (list, dict)) or len(got) != want:
            raise ContractFailure(f'{path}: expected {want} entries')
    for condition in expected.get('find', []):
        items, exists = lookup(value, condition['in'])
        matches = [item for item in items if subset(item, condition['where'])] if exists and isinstance(items, list) else []
        if len(matches) != condition.get('count', 1):
            raise ContractFailure(f"{condition['in']}: unexpected matching entry count {len(matches)}")
        for item in matches:
            check(item, condition.get('expect', {}))
    for path, assertion in expected.get('forEach', {}).items():
        rows, exists = lookup(value, path)
        if not exists or not isinstance(rows, list):
            raise ContractFailure(f'{path}: expected an array for per-entry assertions')
        for row in rows:
            check(row, assertion)
    if 'unitAddresses' in expected:
        got = [row['address'] for group in ('observations', 'failures') for row in value.get(group, [])]
        encode = lambda item: json.dumps(item, sort_keys=True)
        if sorted(map(encode, got)) != sorted(map(encode, expected['unitAddresses'])):
            raise ContractFailure('observations/failures do not account for exactly the requested external units')
    for path, required in expected.get('contains', {}).items():
        got, exists = lookup(value, path)
        if not exists or not isinstance(got, str) or any(word not in got for word in required):
            raise ContractFailure(f'{path}: required text is absent')
    for path, forbidden in expected.get('excludes', {}).items():
        got, exists = lookup(value, path)
        text = json.dumps(got, ensure_ascii=False) if exists else ''
        if any(word in text for word in forbidden):
            raise ContractFailure(f'{path}: contains a forbidden value')


def material_path(root, value):
    if not value.startswith('$materials/'):
        raise HarnessError('material must be relative to this node: $materials/<file>')
    path = (root/value[len('$materials/'):]).resolve()
    if not path.is_relative_to(root.resolve()) or not path.exists():
        raise HarnessError(f'missing or escaping material: {value}')
    return path


def validate_probe(probe):
    steps = probe.get('steps')
    if not isinstance(steps, list) or not steps:
        raise HarnessError(f"{probe.get('id')}: no executable steps")
    if not any(step.get('expect') for step in steps):
        raise HarnessError(f"{probe.get('id')}: command success alone is not an oracle")
    for step in steps:
        if not any(key in step for key in ('kc', 'origin', 'assert', 'writer', 'trial', 'searchSnapshot', 'control', 'go', 'deployment', 'indexDataset')):
            raise HarnessError(f"{probe.get('id')}: unrecognized step")


def run_probe(spec, action):
    row = {'id': spec['id'], 'title': spec['title'], 'startedAt': now()}
    try:
        action()
        row.update(status='pass', reason='all explicit assertions passed')
    except RunInterrupted as exc:
        row.update(status='error', reason=str(exc), interrupted=exc.signum,
                   executionStatus='interrupted' if exc.started else 'not_started')
    except ContractFailure as exc:
        row.update(status='fail', reason=str(exc))
    except Exception as exc:
        row.update(status='error', reason=f'{type(exc).__name__}: {exc}')
    row['finishedAt'] = now()
    row.setdefault('executionStatus', 'completed' if row['status'] in ('pass', 'fail') else 'error')
    return row


def batch(specs, probes, *, preflight, construct):
    blocked = None
    prestate = {'status': 'pass', 'phase': 'preflight'}
    try:
        preflight()
        prestate['phase'] = 'construct'
        construct()
    except RunInterrupted as exc:
        blocked = str(exc)
        prestate.update(status='error', reason=blocked, interrupted=exc.signum)
    except ContractFailure as exc:
        blocked = str(exc)
        prestate.update(status='fail', reason=blocked)
    except Exception as exc:
        blocked = f'{type(exc).__name__}: {exc}'
        prestate.update(status='error', reason=blocked)
    results = []
    for spec in specs:
        not_started = bool(blocked) or spec['id'] not in probes
        if blocked:
            def action():
                raise HarnessError('prestate unavailable; this specification did not start: ' + blocked)
        elif spec['id'] not in probes:
            def action():
                raise HarnessError('no executable probe registered; specification is not execution evidence')
        else:
            action = probes[spec['id']]
        row = run_probe(spec, action)
        if not_started:
            row['executionStatus'] = 'not_started'
        if row.get('interrupted'):
            blocked = row['reason']
        results.append(row)
    return {'prestate': prestate, 'results': results, 'summary': {status: sum(r['status'] == status for r in results)
                                           for status in ('pass', 'fail', 'error')}}


def specifications():
    rows = []
    paths = sorted((QINGHE/'_specs').glob('accept-*.yaml')) + [ATTACHED/'_specs/liveaccess.yaml']
    seen = set()
    for path in paths:
        for spec in load_yaml_file(path):
            if spec['id'] in seen:
                raise HarnessError('duplicate specification id: ' + spec['id'])
            seen.add(spec['id'])
            spec = dict(spec, file=str(path.relative_to(REPO)))
            rows.append(spec)
    return rows


def read_probes():
    if not PROBES.exists():
        return {}
    out = {}
    for probe in json.loads(PROBES.read_text()):
        validate_probe(probe)
        if probe['id'] in out:
            raise HarnessError('duplicate probe id: ' + probe['id'])
        out[probe['id']] = probe
    return out


def get_json(url, method='GET', body=None):
    return read_endpoint(url, method, body, json.load)


def check_http_health(url):
    read_endpoint(url, 'GET', None, lambda response: None)


def read_endpoint(url, method, body, decode):
    data = None if body is None else json.dumps(body).encode()
    request = urllib.request.Request(url, data=data, method=method,
                                     headers={'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(request, timeout=5) as response:
            return decode(response)
    except urllib.error.HTTPError as exc:
        status = exc.code
        exc.close()
        raise HarnessError(f'deployment endpoint unavailable: {url}: HTTP {status}') from None
    except (OSError, ValueError) as exc:
        raise HarnessError(f'deployment endpoint unavailable: {url}: {exc}') from exc


def preflight():
    if os.environ.get('KC_DEPLOY_PROFILE', 'local') != 'local':
        raise HarnessError('acceptance runs only on the local test deployment')
    home = Path(os.environ.get('KC_DEPLOY_HOME', '/tmp/kc-deploy-local'))
    if not (home/'compose.env').is_file():
        raise HarnessError(f'local test stack configuration missing: {home}/compose.env; no services were started')
    for key in ('KC_SERVER_URL', 'KC_ACCEPT_LAKEFS_URL', 'KC_ACCEPT_OPENSEARCH_URL',
                'KC_ACCEPT_ORIGIN_URL', 'KC_ACCEPT_UNTRUSTED_ORIGIN_URL', 'KC_ACCEPT_ALT_ORIGIN_URL'):
        if not os.environ.get(key):
            raise HarnessError(f'{key} is missing; use the local deployment acceptance entry')
    # Check before invoking any Writer, control API, or runtime setup.
    get_json(os.environ['KC_SERVER_URL'].rstrip('/') + '/readyz')
    check_http_health(os.environ['KC_ACCEPT_LAKEFS_URL'].rstrip('/') + '/_health')
    for key in ('KC_ACCEPT_ORIGIN_URL', 'KC_ACCEPT_UNTRUSTED_ORIGIN_URL', 'KC_ACCEPT_ALT_ORIGIN_URL'):
        health = get_json(os.environ[key].rstrip('/') + '/health')
        if health.get('role') != 'live-origin':
            raise HarnessError(f'{key}: endpoint is not the acceptance live-origin')
    search_file = REPO/'scripts/opensearch_fixture.py'
    module_spec = importlib.util.spec_from_file_location('accept_search_fixture', search_file)
    module = importlib.util.module_from_spec(module_spec)
    module_spec.loader.exec_module(module)
    try:
        module.capacity(os.environ['KC_ACCEPT_OPENSEARCH_URL'])
    except (OSError, ValueError) as exc:
        raise HarnessError(str(exc)) from exc



# accept-06/07 specifications that need a Dataset revision to be prepared,
# built per member, or attributed in search results; the declaration task
# cannot make them green before INDEX-SEARCH-SPACE-01 exists.
DATASET_INDEX_SPECS = frozenset({
    'accept-index-writer-accepts-unsupported',
    'accept-index-array-input',
    'accept-search-unsupported-member-isolated',
    'accept-search-vector-member',
    'accept-index-catalog-version-ahead',
    'accept-index-catalog-upgrade-recheck',
})


def owners(spec):
    name, filename = spec['id'], Path(spec['file']).name
    if name in ('accept-cross-descriptor-repo-not-selected', 'accept-retire-bound-schema-path'):
        return ['LIVEACCESS-DECLARATION-01']
    if name in ('accept-cross-request-deadline', 'accept-cross-origin-exact-match'):
        return ['LIVEACCESS-READ-01']
    if name == 'accept-cross-per-caller-not-searchable':
        return ['LIVEACCESS-READ-01', 'INDEX-SEARCH-SPACE-01']
    if name == 'accept-retire-repository-plugin-selection' or filename.startswith('accept-01'):
        return ['PLUGIN-INTERPRETER-01']
    if filename.startswith(('accept-02', 'accept-05')):
        return ['LIVEACCESS-DECLARATION-01']
    if filename.startswith(('accept-03', 'accept-04')):
        return ['LIVEACCESS-READ-01']
    if name in DATASET_INDEX_SPECS:
        return ['INDEX-SEARCH-SPACE-01']
    if filename.startswith(('accept-06', 'accept-07')):
        return ['INDEX-DECLARATION-01']
    if filename.startswith(('accept-08', 'accept-09', 'accept-13')) or name == 'accept-retire-repository-scoped-search':
        return ['INDEX-SEARCH-SPACE-01']
    if filename.startswith('accept-10'):
        return ['INDEX-USAGE-01']
    if filename.startswith('accept-12'):
        return ['DATASET-REVISION-01']
    if name == 'liveaccess-indexed-member':
        return ['INDEX-SEARCH-SPACE-01']
    if name in ('liveaccess-selected-plugin-result', 'liveaccess-replacement-keeps-declaration', 'liveaccess-no-compatible-plugin'):
        return ['PLUGIN-INTERPRETER-01', 'LIVEACCESS-PLUGIN-EXECUTION-01']
    return ['LIVEACCESS-PLUGIN-EXECUTION-01']


def owner(spec):
    return owners(spec)[0]


def publish_report(output, report):
    temporary = output/('.acceptance-'+uuid.uuid4().hex+'.json')
    try:
        with temporary.open('x', encoding='utf-8') as stream:
            json.dump(report, stream, ensure_ascii=False, indent=2)
            stream.write('\n')
        temporary.replace(output/'acceptance.json')
    finally:
        temporary.unlink(missing_ok=True)



def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--list', action='store_true', help='read-only specification/probe inventory')
    parser.add_argument('--check', action='store_true', help='offline coverage/shape check, no services')
    parser.add_argument('--select', action='append', help='explicit specification id; repeatable')
    args = parser.parse_args()
    specs, plans = specifications(), read_probes()
    ids = {s['id'] for s in specs}
    orphaned = set(plans) - ids
    if orphaned:
        raise HarnessError('probes reference absent specifications: ' + ', '.join(sorted(orphaned)))
    if args.select:
        unknown = set(args.select)-ids
        if unknown:
            raise HarnessError('unknown specification selection: ' + ', '.join(sorted(unknown)))
        specs = [s for s in specs if s['id'] in args.select]
    missing = [s['id'] for s in specs if s['id'] not in plans]
    inventory = [{'id': s['id'], 'specification': s['file'], 'probe': s['id'] if s['id'] in plans else None,
                  'executionStatus': 'not_evaluated', 'productOwnerTasks': owners(s), 'harnessOwnerTask': 'ACCEPT-HARNESS-01'} for s in specs]
    if args.list or args.check:
        print(json.dumps({'specifications': len(specs), 'executableProbes': len(specs)-len(missing),
                          'missingProbes': missing, 'inventory': inventory}, ensure_ascii=False, indent=2))
        return int(args.check and bool(missing))
    from accept_world import World
    world = World(plans)
    started = now()
    cancelled = []
    action_phase = None
    previous_handlers = {}
    def interrupt(signum, _frame):
        cancelled.append(signum)
        if action_phase is not None and len(cancelled) == 1:
            raise RunInterrupted(signum, started=action_phase == 'running')
    def guarded(action):
        def run():
            nonlocal action_phase
            action_phase = 'pending'
            try:
                if cancelled:
                    raise RunInterrupted(cancelled[-1], started=False)
                action_phase = 'running'
                return action()
            finally:
                action_phase = None
        return run
    def restore_handlers():
        for signum, handler in previous_handlers.items():
            signal.signal(signum, handler)
    callbacks = {s['id']: guarded(lambda spec_id=s['id']: world.probe(spec_id))
                 for s in specs if s['id'] in plans}
    for signum in (signal.SIGINT, signal.SIGTERM):
        previous_handlers[signum] = signal.signal(signum, interrupt)
    cleanup_error = None
    try:
        try:
            report = batch(specs, callbacks, preflight=guarded(preflight), construct=guarded(world.construct))
        finally:
            # Cleanup is already bounded by World and owns this invocation's
            # resources. Record signals here; do not abort or restart cleanup.
            try:
                world.close()
            except Exception as exc:
                cleanup_error = str(exc)
        report.update(schemaVersion=1, scope='deploy-local-accept', startedAt=started, finishedAt=now(),
                      runId=os.environ.get('KC_VALIDATION_RUN_ID'),
                      sourceFingerprint=os.environ.get('KC_VALIDATION_SOURCE_FINGERPRINT'),
                      missingProbes=missing, inventory=inventory, prefix=world.prefix, cleanupError=cleanup_error,
                      cleanup=world.cleanup_report, events=world.events, interrupted=bool(cancelled))
        for row in report['results']:
            spec = next(spec for spec in specs if spec['id'] == row['id'])
            product_owners = owners(spec)
            row['productOwnerTasks'] = product_owners
            row['ownerTask'] = 'ACCEPT-HARNESS-01' if row['status'] == 'error' else product_owners[0]
            row['requiresDebtRegistration'] = row['status'] == 'fail' and any((REPO/'tasks/done'/(task+'.md')).exists() for task in product_owners)
        output = Path(os.environ.get('KC_VALIDATION_RUN_DIR', REPO/'.validation/runs'/('accept-'+uuid.uuid4().hex)))
        output.mkdir(parents=True, exist_ok=True)
        publish_report(output, report)
        # Cancellation only moves false -> true. One replacement suffices if
        # the first signal arrived while the initial report was being written.
        if cancelled and not report['interrupted']:
            report['interrupted'] = True
            publish_report(output, report)
        exit_code = 128+cancelled[-1] if cancelled else int(
            bool(cleanup_error) or any(report['summary'][s] for s in ('fail', 'error')))
        # The atomically published result is the completion boundary. Restore
        # ordinary signal behavior before announcing it; a later termination
        # does not retroactively interrupt completed acceptance actions.
        restore_handlers()
        print(json.dumps({'report': str(output/'acceptance.json'), 'summary': report['summary']}))
        return exit_code
    finally:
        restore_handlers()


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except (HarnessError, KeyError, OSError, ValueError) as error:
        print('accept: ' + str(error), file=sys.stderr)
        raise SystemExit(2)
