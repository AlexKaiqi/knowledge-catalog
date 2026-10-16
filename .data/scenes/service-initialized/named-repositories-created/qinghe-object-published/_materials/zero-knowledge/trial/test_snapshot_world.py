import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


class SupplierRepositorySetupTest(unittest.TestCase):
    def test_setup_scopes_writes_and_publication_to_configured_repositories(self):
        cases = (
            ({}, ["table-meta", "sales-semantic"]),
            ({"KC_ZK_TABLE_REPOSITORY": "supplier-table-meta",
              "KC_ZK_SEMANTIC_REPOSITORY": "supplier-sales-semantic"},
             ["supplier-table-meta", "supplier-sales-semantic"]),
        )
        for overrides, repositories in cases:
            with self.subTest(repositories=repositories), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                calls = root / "docker-calls.jsonl"
                docker = root / "docker"
                docker.write_text("#!" + sys.executable + "\n" + """
import json, os, sys
args = sys.argv[1:]
command = args[args.index('/usr/local/bin/kc') + 2:]
with open(os.environ['TEST_DOCKER_CALLS'], 'a') as output:
    output.write(json.dumps(command) + '\\n')
if command == ['show']:
    print(json.dumps({'repositories': [], 'datasets': []}))
elif command[:2] == ['grant', 'list']:
    print(json.dumps({'rules': []}))
else:
    print('{}')
""")
                docker.chmod(0o755)
                environment = {key: value for key, value in os.environ.items() if not key.startswith('KC_ZK_')}
                environment.update(PATH=str(root) + os.pathsep + environment['PATH'],
                                   TEST_DOCKER_CALLS=str(calls), **overrides)
                result = subprocess.run(['bash', str(Path(__file__).with_name('snapshot-world.sh')), 'setup'],
                                        env=environment, capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                commands = [json.loads(line) for line in calls.read_text().splitlines()]
                created = [command[command.index('--name') + 1] for command in commands if command[:1] == ['create']]
                self.assertEqual(created, repositories)
                scoped = [command[command.index('--repo') + 1] for command in commands if '--repo' in command]
                self.assertEqual(set(scoped), set(repositories))
                publication = [command for command in commands if 'repository.dataset.publish' in command]
                self.assertEqual(len(publication), 2)
                self.assertTrue(all(command[command.index('--dataset') + 1] == 'qinghe-shared' for command in publication))


if __name__ == '__main__':
    unittest.main()
