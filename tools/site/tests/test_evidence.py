"""Exercise the stale-data gate in an isolated copy, never modify real reports."""
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]


class EvidenceCheck(unittest.TestCase):
    def test_source_change_and_output_corruption_are_detected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config = json.loads((ROOT / 'tools/site/evidence.json').read_text())
            kinds = ('retrieval', 'generation', 'external', 'verification', 'indexing')
            for name in ('tools/site/build_evidence.py', 'tools/site/evidence.json',
                         'go.mod', *(config[kind] for kind in kinds)):
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(ROOT / name, target)
            def run(*args):
                return subprocess.run([sys.executable, str(root / 'tools/site/build_evidence.py'), *args], capture_output=True)
            self.assertEqual(run().returncode, 0)
            self.assertEqual(run('--check').returncode, 0)
            for kind in kinds:
                with self.subTest(kind=kind):
                    report_path = root / config[kind]
                    report = json.loads(report_path.read_text())
                    report['test_drift_marker'] = True
                    report_path.write_text(json.dumps(report))
                    self.assertNotEqual(run('--check').returncode, 0)
                    self.assertEqual(run().returncode, 0)
                    self.assertEqual(run('--check').returncode, 0)
                    snapshot = root / f'site/data/{kind}.json'
                    self.assertTrue(json.loads(snapshot.read_text())['test_drift_marker'])
                    snapshot.write_text('{}')
                    self.assertNotEqual(run('--check').returncode, 0)
                    self.assertEqual(run().returncode, 0)
            report_path = root / config['retrieval']
            report = json.loads(report_path.read_text())
            report['aggregate']['hit_at_5'] = 0.5
            report_path.write_text(json.dumps(report))
            self.assertNotEqual(run().returncode, 0, 'inconsistent summary must be rejected')


if __name__ == '__main__':
    unittest.main()
