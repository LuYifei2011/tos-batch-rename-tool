#!/usr/bin/env python3
"""Scan locked npm dependencies, Linux Go sources and packaged ELF binaries."""
import argparse
import io
from pathlib import Path
import subprocess
import tarfile
import tempfile
import re
from toolchain import ROOT, release_go

parser = argparse.ArgumentParser()
parser.add_argument("packages", nargs="*", type=Path)
parser.add_argument("--go", default="go")
args = parser.parse_args()
env, go_version = release_go(args.go)
subprocess.run(["npm", "audit", "--audit-level=low"], cwd=ROOT, check=True)

with tempfile.TemporaryDirectory(prefix="batch-rename-security-") as tmp:
    scanner = Path(tmp) / "govulncheck"
    scanner_version = (ROOT / ".govulncheck-version").read_text().strip()
    subprocess.run([args.go, "install", "golang.org/x/vuln/cmd/govulncheck@" + scanner_version],
                   cwd=ROOT, env=dict(env, GOBIN=tmp), check=True)
    for arch in ("amd64", "arm64"):
        print(f"Scanning Linux {arch} source", flush=True)
        subprocess.run([str(scanner), "./server"], cwd=ROOT,
                       env=dict(env, GOOS="linux", GOARCH=arch, CGO_ENABLED="0"), check=True)
    for index, package in enumerate(args.packages):
        data = package.read_bytes()
        if data[:8] != b"!<arch>\n":
            parser.error(f"Not a deb archive: {package}")
        pos = 8
        payload = None
        while pos < len(data):
            header = data[pos:pos + 60]
            size = int(header[48:58])
            if header[:16].decode().strip().rstrip("/") == "data.tar.gz":
                payload = data[pos + 60:pos + 60 + size]
            pos += 60 + size + size % 2
        if payload is None:
            parser.error(f"Missing data.tar.gz: {package}")
        with tarfile.open(fileobj=io.BytesIO(payload)) as archive:
            binary = Path(tmp) / f"binary-{index}"
            binary.write_bytes(archive.extractfile("./usr/local/batch-rename/bin/batch-rename").read())
        print(f"Scanning packaged binary: {package.name}", flush=True)
        build_info = subprocess.check_output([args.go, "version", "-m", str(binary)], env=env, text=True)
        if not re.search(r":\s+" + re.escape(go_version) + r"\s*$", build_info.splitlines()[0]):
            parser.error(f"Packaged binary was not compiled with {go_version}: {package}")
        subprocess.run([str(scanner), "-mode=binary", str(binary)], cwd=ROOT, env=env, check=True)
