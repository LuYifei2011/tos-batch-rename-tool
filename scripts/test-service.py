#!/usr/bin/env python3
"""Exercise the packaged service on an isolated GitHub Actions Linux runner."""
import grp
import http.client
import os
from pathlib import Path
import pwd
import shutil
import socket
import subprocess
import sys
import time


def run(*args, check=True):
    result = subprocess.run(args, text=True, capture_output=True)
    if check and result.returncode:
        raise RuntimeError(f"{' '.join(args)} failed ({result.returncode})\n{result.stdout}{result.stderr}")
    return result


def wait_state(expected):
    for _ in range(100):
        state = run("systemctl", "is-active", "batch-rename.service", check=False).stdout.strip()
        if state == expected:
            return
        time.sleep(0.1)
    raise AssertionError(f"service state is {state}, expected {expected}")


if sys.platform != "linux" or os.geteuid() != 0 or os.environ.get("GITHUB_ACTIONS") != "true":
    sys.exit("Run only as root on an isolated GitHub Actions Linux runner")
app = Path("/usr/local/batch-rename")
if app.exists() or app.is_symlink() or run("id", "batch-rename", check=False).returncode == 0:
    sys.exit("Refusing to replace an existing application or account")
package = str(Path(sys.argv[1]).resolve())
created_account = False
created_primary_group = False
primary_group = "batch-rename-runtime"
for name in ("batch-rename", primary_group):
    if run("getent", "group", name, check=False).returncode == 0:
        sys.exit(f"Refusing to replace existing group {name}")
try:
    run("dpkg", "--install", package)
    assert not (app / "data").exists(), "installation must defer permissions until the account exists"
    assert (app / "site/index.html").is_file()
    installed_gid = grp.getgrnam("batch-rename").gr_gid
    assert installed_gid > 0, "postinst must create the group before the TOS user exists"
    assert run("id", "batch-rename", check=False).returncode != 0
    # Match TOS accounts whose primary group differs from the dedicated group.
    run("groupadd", "--system", primary_group)
    created_primary_group = True
    run("useradd", "--system", "--no-user-group", "--gid", primary_group, "--no-create-home", "--shell", "/usr/sbin/nologin", "batch-rename")
    created_account = True
    account = pwd.getpwnam("batch-rename")
    assert account.pw_gid == grp.getgrnam(primary_group).gr_gid
    assert installed_gid != account.pw_gid
    run("systemctl", "reset-failed", "batch-rename.service", "batch-rename.socket")
    run("systemctl", "start", "batch-rename.service")
    wait_state("active")
    conn = http.client.HTTPConnection("localhost", timeout=5)
    conn.sock = socket.socket(socket.AF_UNIX)
    conn.sock.settimeout(5)
    conn.sock.connect("/var/api/batch-rename.sock")
    conn.request("GET", "/api/info")
    response = conn.getresponse()
    assert response.status == 401, "socket must serve HTTP with authentication enforced"
    response.read()
    conn.close()
    data = app / "data"
    assert data.stat().st_uid == account.pw_uid and data.stat().st_mode & 0o777 == 0o700
    pid = int(run("systemctl", "show", "--property=MainPID", "--value", "batch-rename.service").stdout)
    assert Path(f"/proc/{pid}").stat().st_uid == account.pw_uid, "business process must be non-root"
    assert Path(f"/proc/{pid}").stat().st_gid == installed_gid
    assert data.stat().st_gid == account.pw_gid
    assert Path("/var/api/batch-rename.sock").stat().st_uid == account.pw_uid
    assert Path("/var/api/batch-rename.sock").stat().st_gid == installed_gid
    assert Path("/var/api/batch-rename.sock").stat().st_mode & 0o777 == 0o660
    run("systemctl", "stop", "batch-rename.service")
    assert not Path("/var/api/batch-rename.sock").exists()
    record = data / "history/0123456789abcdef0123456789abcdef.json"
    record.write_text("damaged history")
    run("systemctl", "start", "batch-rename.service", check=False)
    wait_state("failed")
    assert record.read_text() == "damaged history"
    assert run("systemctl", "show", "--property=NRestarts", "--value", "batch-rename.service").stdout.strip() == "0"
    record.unlink()
    run("systemctl", "reset-failed", "batch-rename.service", "batch-rename.socket")
    run("systemctl", "start", "batch-rename.service")
    wait_state("active")
    (app / "site/stale.js").write_text("old frontend")
    run("dpkg", "--install", package)
    wait_state("active")
    assert grp.getgrnam("batch-rename").gr_gid == installed_gid
    assert not (app / "site/stale.js").exists()
    record.write_text("retain this record")
    run("dpkg", "--remove", "batch-rename")
    run("dpkg", "--purge", "batch-rename")
    assert not (app / "site").exists()
    assert not Path("/var/api/batch-rename.sock").exists()
    assert record.read_text() == "retain this record"
    assert run("systemctl", "is-enabled", "batch-rename.service", check=False).returncode != 0
    assert run("systemctl", "is-enabled", "batch-rename.socket", check=False).returncode != 0
    print("PASS systemd: install-time group provisioning, delayed account, dedicated GID, socket ownership, root initialization, non-root HTTP, recovery, upgrade and removal")
except Exception:
    print(run("journalctl", "-u", "batch-rename.service", "-n", "80", "--no-pager", check=False).stdout, file=sys.stderr)
    raise
finally:
    run("systemctl", "stop", "batch-rename.service", "batch-rename.socket", check=False)
    run("dpkg", "--purge", "batch-rename", check=False)
    if created_account:
        run("userdel", "batch-rename", check=False)
    if created_primary_group:
        run("groupdel", primary_group, check=False)
    # Uninstall retains the group; remove it only in this isolated test fixture.
    run("groupdel", "batch-rename", check=False)
    if app.is_dir() and not app.is_symlink():
        shutil.rmtree(app)
