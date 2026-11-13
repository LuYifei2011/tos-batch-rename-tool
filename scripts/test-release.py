#!/usr/bin/env python3
"""Exercise metadata failures that must block a release."""
import importlib.util
import json
from pathlib import Path
import shutil
import tempfile
import unittest
import os
import subprocess
import toolchain

ROOT = Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location("metadata", ROOT / "scripts/check-metadata.py")
metadata = importlib.util.module_from_spec(spec)
spec.loader.exec_module(metadata)


class ReleaseMetadataTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory(prefix="batch-rename-release-")
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        (self.root / "packaging").mkdir()
        for name in ("package.json", "package-lock.json", "LICENSE", "packaging/config.ini", "packaging/batch-rename.lang"):
            shutil.copy(ROOT / name, self.root / name)

    def update_json(self, name, change):
        path = self.root / name
        data = json.loads(path.read_text())
        change(data)
        path.write_text(json.dumps(data))

    def test_current_metadata_and_matching_tag(self):
        version = json.loads((self.root / "package.json").read_text())["version"]
        self.assertEqual(metadata.validate(self.root, "v" + version)["version"], version)

    def test_missing_help_is_rejected(self):
        self.update_json("packaging/config.ini", lambda data: data.pop("help"))
        with self.assertRaisesRegex(ValueError, "help"):
            metadata.validate(self.root)
    def test_lockfile_version_drift_is_rejected(self):
        self.update_json("package-lock.json", lambda data: data.update(version="99.0.0"))
        with self.assertRaisesRegex(ValueError, "versions differ"):
            metadata.validate(self.root)

    def test_wrong_tag_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "Tag must"):
            metadata.validate(self.root, "v99.0.0")

    def test_publisher_drift_is_rejected(self):
        self.update_json("packaging/config.ini", lambda data: data.update(publisher="Someone Else"))
        with self.assertRaisesRegex(ValueError, "Publisher"):
            metadata.validate(self.root)

    def test_missing_language_is_rejected(self):
        path = self.root / "packaging/batch-rename.lang"
        path.write_text(path.read_text().replace("[he-il]", "[other]"))
        with self.assertRaisesRegex(ValueError, "26 language"):
            metadata.validate(self.root)

    def test_missing_license_is_rejected(self):
        (self.root / "LICENSE").unlink()
        with self.assertRaises(OSError):
            metadata.validate(self.root)

    def test_repository_mismatch_is_rejected(self):
        self.update_json("packaging/config.ini", lambda data: data.update(help="https://github.com/example/other"))
        with self.assertRaisesRegex(ValueError, "Repository URLs differ"):
            metadata.validate(self.root)


class ReleaseToolchainTests(unittest.TestCase):
    def test_selected_toolchain_matches_release_pin(self):
        env, actual = toolchain.release_go()
        self.assertEqual(actual, "go" + (ROOT / ".go-version").read_text().strip())
        self.assertEqual(env["GOTOOLCHAIN"], "local")

    def test_old_toolchain_is_rejected_before_build(self):
        with tempfile.TemporaryDirectory(prefix="batch-rename-go-") as tmp:
            go = Path(tmp) / "go"
            go.write_text("#!/bin/sh\nprintf 'go1.27.3\\n'\n")
            go.chmod(0o755)
            result = subprocess.run([os.sys.executable, str(ROOT / "scripts/build-package.py"), "amd64", "--go", str(go)],
                                    text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("Release builds require go1.27.4", result.stderr)


if __name__ == "__main__":
    unittest.main(verbosity=2)
