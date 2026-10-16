#!/usr/bin/env python3
"""Check shared search capacity and clean only this invocation's owned prefix."""
import argparse
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

PREFIX = re.compile(r'kc-test-[0-9a-f]{32}-\Z')


SCENE_TESTS = frozenset({
    'TestProductScenes', 'TestMetricPermissionScenes',
    'TestSceneProjectionNamespaceRewritesOnlyPhysicalCoordinates',
    'TestSceneProjectionNamespaceForkIsolatesFrozenMedia',
    'TestSceneProjectionNamespaceUsesInvocationOwnership',
    'TestSceneProjectionCloneFailureStillCleansCreatedTarget',
})


def require_scene_command(command):
    """Only these entry points bind every physical index to the owned prefix.

    Home/CLI component fixtures use hashed provider identities instead. Passing
    the environment variable to them cannot establish cleanup ownership.
    """
    error = ('shared OpenSearch run requires the scene namespace transport: '
             'scripts/testsuite.sh lakefs or explicit supported scene tests in ./cli; '
             'ordinary Go fixtures require an exclusive OpenSearch instance')
    suite = Path(__file__).resolve().with_name('testsuite.sh')
    if len(command) == 2 and Path(command[0]).resolve() == suite and command[1] == 'lakefs':
        return
    if len(command) < 3 or Path(command[0]).name != 'go' or command[1] != 'test':
        raise ValueError(error)
    args = iter(command[2:])
    selectors, packages = [], []
    for argument in args:
        if argument == '-run':
            selectors.append(next(args, ''))
        elif argument.startswith('-run='):
            selectors.append(argument[5:])
        elif argument in ('-v', '-short', '-json'):
            continue
        elif argument in ('-count', '-timeout'):
            if not next(args, ''):
                raise ValueError(error)
        elif re.fullmatch(r'-(?:count|timeout)=[A-Za-z0-9.]+', argument):
            continue
        elif argument == './cli':
            packages.append(argument)
        else:
            raise ValueError(error)
    if len(selectors) != 1 or packages != ['./cli']:
        raise ValueError(error)
    expression = selectors[0]
    if not expression.startswith('^') or not expression.endswith('$'):
        raise ValueError(error)
    expression = expression[1:-1]
    if expression.startswith('(') and expression.endswith(')'):
        expression = expression[1:-1]
    if not expression or not set(expression.split('|')) <= SCENE_TESTS:
        raise ValueError(error)


def request(endpoint, method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(endpoint.rstrip('/') + path, method=method, data=data,
                                 headers={'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            return json.load(response)
    except urllib.error.HTTPError as exc:
        status = exc.code
        exc.close()
        raise OSError(f'OpenSearch API {method} returned HTTP {status}') from None


def capacity(endpoint, required=8):
    """Count assigned and unassigned open shards against the cluster's limit."""
    settings = request(endpoint, 'GET', '/_cluster/settings?include_defaults=true&flat_settings=true')
    limit = 1000
    for scope in ('defaults', 'persistent', 'transient'):
        values = settings.get(scope, {})
        value = values.get('cluster.max_shards_per_node', values.get('cluster', {}).get('max_shards_per_node'))
        if value is not None:
            limit = int(value)
    nodes = request(endpoint, 'GET', '/_nodes?filter_path=nodes.*.roles')
    data_nodes = sum(any(role == 'data' or role.startswith('data_') and role != 'data_frozen'
                         for role in node.get('roles', [])) for node in nodes.get('nodes', {}).values())
    if not data_nodes:
        raise ValueError('OpenSearch capacity: no eligible data nodes; no tests were started')
    maximum = limit * data_nodes
    # Open-index metadata counts primaries and configured replicas, including
    # initializing/unassigned copies, without double-counting relocations.
    indices = request(endpoint, 'GET', '/_all/_settings?expand_wildcards=open&flat_settings=true')
    used = 0
    for index in indices.values():
        settings = index['settings']
        nested = settings.get('index', {})
        primaries = int(settings.get('index.number_of_shards', nested.get('number_of_shards', 1)))
        replicas = int(settings.get('index.number_of_replicas', nested.get('number_of_replicas', 1)))
        used += primaries * (replicas + 1)
    available = maximum - used
    if available < required:
        raise ValueError(f'OpenSearch capacity: {used}/{maximum} open shards, '
                         f'{available} free; this test run requires at least {required}. '
                         'No shared indices were deleted and no shard limit was changed.')
    return {'openShards': used, 'maximumShards': maximum, 'freeShards': available}


def cleanup(endpoint, prefix):
    """Enumerate the prefix, then delete exact names; never send wildcard DELETE."""
    if not PREFIX.fullmatch(prefix):
        raise ValueError('refusing cleanup without a complete invocation-owned prefix')
    path = '/_cat/indices/' + prefix + '*?format=json&h=index&expand_wildcards=all'
    rows = request(endpoint, 'GET', path)
    cleaned = []
    for name in sorted({row['index'] for row in rows}):
        if not name.startswith(prefix):
            continue
        request(endpoint, 'DELETE', '/' + urllib.parse.quote(name, safe=''))
        cleaned.append(name)
    return cleaned


def run_owned(endpoint, command, required=8, grace_seconds=2):
    capacity(endpoint, required)
    prefix = 'kc-test-' + uuid.uuid4().hex + '-'
    child = None
    previous = {}
    status = 1
    interrupted = []
    interruption_time = None

    def forward(signum):
        if child is not None and child.poll() is None:
            try:
                os.killpg(child.pid, signum)
            except ProcessLookupError:
                pass

    def interrupt(signum, _frame):
        nonlocal interruption_time
        interrupted.append(signum)
        if interruption_time is None:
            interruption_time = time.monotonic()
        forward(signum)

    try:
        for signum in (signal.SIGINT, signal.SIGTERM):
            previous[signum] = signal.signal(signum, interrupt)
        env = dict(os.environ, KC_TEST_OPENSEARCH_RUN_PREFIX=prefix)
        child = subprocess.Popen(command, env=env, start_new_session=True)
        # A signal can arrive after installing the handler but before Popen
        # returns. Forward the remembered signal once the process group exists.
        if interrupted:
            forward(interrupted[-1])
        while True:
            try:
                status = child.wait(timeout=0.1)
                break
            except subprocess.TimeoutExpired:
                if interruption_time is not None and time.monotonic()-interruption_time >= grace_seconds:
                    forward(signal.SIGKILL)
                    status = child.wait(timeout=2)
                    break
        if interrupted:
            status = 128 + interrupted[-1]
        elif status < 0:
            status = 128 - status
    finally:
        # Run outside Go's process: t.Cleanup cannot run after SIGTERM/crash.
        try:
            cleanup(endpoint, prefix)
        except (OSError, ValueError) as exc:
            print(f'FAIL: owned OpenSearch cleanup failed: {exc}', file=sys.stderr)
            status = status or 1
        for signum, handler in previous.items():
            signal.signal(signum, handler)
    return 128+interrupted[-1] if interrupted else status


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('run', 'capacity'))
    parser.add_argument('--endpoint', required=True)
    parser.add_argument('--minimum-free-shards', type=int, default=8)
    args, command = parser.parse_known_args()
    if args.minimum_free_shards < 1:
        parser.error('--minimum-free-shards must be positive')
    command = command[1:] if command[:1] == ['--'] else command
    try:
        if args.action == 'capacity':
            print(json.dumps(capacity(args.endpoint, args.minimum_free_shards)))
            return 0
        if not command:
            parser.error('run requires a child command after --')
        require_scene_command(command)
        return run_owned(args.endpoint, command, args.minimum_free_shards)
    except (OSError, ValueError) as exc:
        print(f'FAIL: {exc}', file=sys.stderr)
        return 2


if __name__ == '__main__':
    raise SystemExit(main())
