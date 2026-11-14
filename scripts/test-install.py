#!/usr/bin/env python3
"""Exercise lifecycle hooks with an isolated filesystem and a systemd model."""
import io
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent


class InstallTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory(prefix="batch-rename-install-")
        self.addCleanup(temp.cleanup)
        self.base = Path(temp.name)
        self.app, self.units, self.bin = (self.base / name for name in ("app", "systemd", "bin"))
        for path in (self.app, self.units, self.bin):
            path.mkdir()
        shutil.copytree(ROOT / "packaging/init.d", self.app / "init.d")
        self.archive({"index.html": b"new frontend", "assets/new.js": b"new asset"})
        self.log = self.base / "commands.log"
        self.env = dict(os.environ, PATH=f"{self.bin}:{Path(shutil.which('python3')).parent}:/usr/bin:/bin",
                        MOCK_ACCOUNT="0", MOCK_LOG=str(self.log), MOCK_UNITS=str(self.units))
        self.command("id", 'test "$MOCK_ACCOUNT" = 1')
        self.python_command("systemctl", '''
import os,pathlib,sys
a=sys.argv[1:]; units=pathlib.Path(os.environ['MOCK_UNITS'])
with open(os.environ['MOCK_LOG'],'a') as f: f.write('systemctl '+' '.join(a)+'\\n')
if a[0]=='show':
 p=units/a[-1]
 if '--property=LoadState' in a: print('loaded' if p.exists() else 'not-found')
 elif '--property=FragmentPath' in a: print(p.resolve() if p.is_symlink() else p)
elif a[0]=='enable':
 for name in a[1:]:
  source=pathlib.Path(name); target=units/source.name
  if target.is_symlink(): target.unlink()
  target.symlink_to(source)
elif a[0]=='disable':
 for name in a[1:]:
  p=units/name
  if p.is_symlink(): p.unlink()
elif a[0]=='start':
 if os.environ['MOCK_ACCOUNT']!='1': sys.exit('account missing')
''')
        self.python_command("sha256sum", '''
import hashlib,sys
for name in sys.argv[1:]: print(hashlib.sha256(open(name,'rb').read()).hexdigest()+'  '+name)
''')

    def archive(self, files):
        with tarfile.open(self.app / "webui.bz2", "w:bz2") as archive:
            for name, data in files.items():
                info = tarfile.TarInfo(name)
                info.mode = 0o644
                info.size = len(data)
                archive.addfile(info, io.BytesIO(data))

    def command(self, name, body):
        path = self.bin / name
        path.write_text("#!/bin/sh\nset -eu\n" + body + "\n")
        path.chmod(0o755)

    def python_command(self, name, body):
        path = self.bin / name
        path.write_text("#!" + shutil.which("python3") + "\n" + body)
        path.chmod(0o755)

    def hook(self, name, *args, ok=True):
        text = (ROOT / "packaging/DEBIAN" / name).read_text().replace("/usr/local/batch-rename", str(self.app))
        text = text.replace("/var/api/batch-rename.sock", str(self.base / "app.sock"))
        result = subprocess.run(["/bin/bash", "-s", "--", *args], input=text, env=self.env, text=True, capture_output=True)
        if ok:
            self.assertEqual(result.returncode, 0, result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0)
        return result

    def test_install_defers_start_until_platform_account_exists(self):
        result = self.hook("postinst", "configure")
        self.assertIn("TOS to create", result.stderr)
        self.assertEqual((self.app / "site/index.html").read_bytes(), b"new frontend")
        self.assertFalse((self.app / "data").exists())
        self.assertEqual(len(list(self.units.iterdir())), 2)
        self.assertTrue(all(path.is_symlink() for path in self.units.iterdir()))
        self.assertNotIn("systemctl start", self.log.read_text())
        self.env["MOCK_ACCOUNT"] = "1"
        self.hook("postinst", "configure")
        self.assertIn("systemctl start batch-rename.service", self.log.read_text())

    def test_upgrade_replaces_assets_and_preserves_records(self):
        self.env["MOCK_ACCOUNT"] = "1"
        history = self.app / "data/history"
        history.mkdir(parents=True)
        record = history / "existing.json"
        record.write_text('{"state":"interrupted"}')
        self.hook("postinst", "configure")
        (self.app / "site/assets/old.js").write_text("old")
        self.archive({"index.html": b"upgraded", "assets/current.js": b"current"})
        self.hook("prerm", "upgrade")
        self.hook("postinst", "configure", "0.0.1")
        self.assertEqual((self.app / "site/index.html").read_bytes(), b"upgraded")
        self.assertEqual(list((self.app / "site/assets").iterdir()), [self.app / "site/assets/current.js"])
        self.assertEqual(record.read_text(), '{"state":"interrupted"}')
        self.assertFalse((self.app / "site.previous").exists())
        self.assertFalse((self.app / "site.next").exists())

    def test_failed_extraction_preserves_working_frontend(self):
        self.hook("postinst", "configure")
        for corrupt in (True, False):
            if corrupt:
                (self.app / "webui.bz2").write_bytes(b"not a tar archive")
            else:
                self.archive({"assets/only.js": b"missing index"})
            self.hook("postinst", "configure", ok=False)
            self.assertEqual((self.app / "site/index.html").read_bytes(), b"new frontend")
            self.assertFalse((self.app / "site.next").exists())

    def test_recovery_after_interrupted_frontend_swap(self):
        self.hook("postinst", "configure")
        (self.app / "site").rename(self.app / "site.previous")
        (self.app / "webui.bz2").write_bytes(b"broken upgrade")
        self.hook("postinst", "configure", ok=False)
        self.assertEqual((self.app / "site/index.html").read_bytes(), b"new frontend")

    def test_install_rejects_symlink_paths(self):
        external = self.base / "external"
        external.mkdir()
        marker = external / "keep"
        marker.write_text("user data")
        for name in ("data", "site", "site.next", "site.previous"):
            with self.subTest(name=name):
                path = self.app / name
                path.symlink_to(external, target_is_directory=True)
                self.hook("postinst", "configure", ok=False)
                self.assertEqual(marker.read_text(), "user data")
                path.unlink()

    def test_remove_and_repeated_purge_clean_generated_files_only(self):
        history = self.app / "data/history"
        history.mkdir(parents=True)
        record = history / "existing.json"
        record.write_text("keep")
        self.hook("postinst", "configure")
        sock = socket.socket(socket.AF_UNIX)
        self.addCleanup(sock.close)
        sock.bind(str(self.base / "app.sock"))
        self.hook("prerm", "remove")
        shutil.rmtree(self.app / "init.d")
        for action in ("remove", "purge", "purge"):
            self.hook("postrm", action)
            self.assertFalse((self.app / "site").exists())
            self.assertFalse((self.base / "app.sock").exists())
            self.assertEqual(record.read_text(), "keep")
        self.assertEqual(list(self.units.iterdir()), [])
        self.hook("prerm", "remove")

    def test_uninstall_unlinks_site_symlink_without_touching_target(self):
        external = self.base / "external"
        external.mkdir()
        (external / "keep").write_text("user data")
        (self.app / "site").symlink_to(external, target_is_directory=True)
        self.hook("postrm", "purge")
        self.assertFalse((self.app / "site").is_symlink())
        self.assertEqual((external / "keep").read_text(), "user data")

    def test_migration_from_copied_units(self):
        for source in (ROOT / "scripts/fixtures/legacy-units").iterdir():
            shutil.copy(source, self.units / source.name)
        self.hook("postinst", "configure", "0.1.0")
        self.assertEqual(len(list(self.units.iterdir())), 2)
        self.assertTrue(all(path.is_symlink() for path in self.units.iterdir()))

    def test_custom_units_require_explicit_migration(self):
        unit = self.units / "batch-rename.service"
        unit.write_text("custom service")
        result = self.hook("postinst", "configure", ok=False)
        self.assertIn("manual migration", result.stderr)
        self.assertEqual(unit.read_text(), "custom service")


if __name__ == "__main__":
    unittest.main(verbosity=2)
