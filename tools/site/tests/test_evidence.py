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
            for name in ('tools/site/build_evidence.py', 'tools/site/evidence.json',
                         'docs/eval/baseline_v8.json',
                         'docs/eval/baseline_v7_structural_answer_al5.json', 'go.mod'):
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(ROOT / name, target)
            def run(*args):
                return subprocess.run([sys.executable, str(root / 'tools/site/build_evidence.py'), *args], capture_output=True)
            self.assertEqual(run().returncode, 0)
            self.assertEqual(run('--check').returncode, 0)
            report_path = root / 'docs/eval/baseline_v8.json'
            report = json.loads(report_path.read_text())
            report['aggregate']['hit_at_5'] = 0.5
            report_path.write_text(json.dumps(report))
            self.assertNotEqual(run('--check').returncode, 0)
            self.assertEqual(run().returncode, 0)
            self.assertEqual(run('--check').returncode, 0)
            snapshot = root / 'site/data/retrieval.json'
            self.assertEqual(json.loads(snapshot.read_text())['aggregate']['hit_at_5'], 0.5)
            snapshot.write_text('{}')
            self.assertNotEqual(run('--check').returncode, 0)


if __name__ == '__main__':
    unittest.main()
