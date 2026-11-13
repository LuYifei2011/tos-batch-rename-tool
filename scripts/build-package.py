#!/usr/bin/env python3
"""Build a TOS deb preview using Python's tarfile and an explicit ar writer."""
import argparse
import io
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import hashlib
from toolchain import release_go

ROOT = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser()
parser.add_argument("arch", choices=["amd64", "arm64"])
parser.add_argument("--go", default="go")
args = parser.parse_args()
go_env, go_version = release_go(args.go)
subprocess.run([sys.executable, str(ROOT / "scripts/check-metadata.py")], check=True)
subprocess.run(["npm", "run", "build"], cwd=ROOT, check=True)
stage = ROOT / "build" / f"stage-{args.arch}"
if stage.exists():
    shutil.rmtree(stage)
app = stage / "usr/local/batch-rename"
(app / "bin").mkdir(parents=True)
config = json.loads((ROOT / "packaging/config.ini").read_text())
version = config["version"]
package = json.loads((ROOT / "package.json").read_text())
env = dict(go_env, GOOS="linux", GOARCH=args.arch, CGO_ENABLED="0")
subprocess.run([args.go, "build", "-trimpath", f"-ldflags=-s -w -X main.appVersion={version}", "-o", str(app / "bin/batch-rename"), "./server"], cwd=ROOT, env=env, check=True)
(app / "build-info.json").write_text(json.dumps({
    "version": version, "go": go_version, "os": "linux", "arch": args.arch,
    "binarySha256": hashlib.sha256((app / "bin/batch-rename").read_bytes()).hexdigest(),
}, indent=2) + "\n")
config["platform"] = "x86_64" if args.arch == "amd64" else "aarch64"
(app / "config.ini").write_text(json.dumps(config, indent=2) + "\n")
shutil.copy(ROOT / "packaging/batch-rename.lang", app)
shutil.copytree(ROOT / "packaging/init.d", app / "init.d")
(app / "images/icons").mkdir(parents=True)
shutil.copy(ROOT / "src/assets/rename.svg", app / "images/icons/batch-rename.svg")
with tarfile.open(app / "webui.bz2", "w:bz2", format=tarfile.USTAR_FORMAT) as tar:
    for p in sorted((ROOT / "dist").rglob("*")):
        tar.add(p, arcname=str(p.relative_to(ROOT / "dist")), recursive=False)
(app / "docs").mkdir()
for name in ["README.md", "THIRD_PARTY_NOTICES.md", "LICENSE"]:
    shutil.copy(ROOT / name, app)
for name in ["COMPATIBILITY.md", "DEVELOPMENT.md", "RELEASE_NOTES.md", "DESIGN.md"]:
    shutil.copy(ROOT / "docs" / name, app / "docs" / name)
licenses = app / "licenses"
licenses.mkdir()
for name in ["vue", "@vue/shared", "@vue/reactivity", "@vue/runtime-core", "@vue/runtime-dom"]:
    shutil.copy(ROOT / "node_modules" / name / "LICENSE", licenses / (name.replace("/", "-").replace("@", "") + ".txt"))
goroot = Path(subprocess.check_output([args.go, "env", "GOROOT"], env=env, text=True).strip())
# Official tarballs keep LICENSE in GOROOT. Homebrew moves it from libexec to the keg root.
candidates = [goroot / "LICENSE"]
if goroot.name == "libexec":
    candidates.append(goroot.parent / "LICENSE")
go_license = next((path for path in candidates if path.is_file()), None)
if go_license is None or "The Go Authors" not in go_license.read_text(encoding="utf-8"):
    sys.exit(f"Go LICENSE not found for GOROOT {goroot}")
shutil.copy(go_license, licenses / "go.txt")
module = json.loads(subprocess.check_output([args.go,"list","-m","-json","golang.org/x/sys"],cwd=ROOT,env=env,text=True))
shutil.copy(Path(module["Dir"]) / "LICENSE", licenses / "x-sys.txt")

control = stage / "DEBIAN"
shutil.copytree(ROOT / "packaging/DEBIAN", control)
(control / "control").write_text(f"""Package: batch-rename
Version: {version}
Section: utils
Priority: optional
Architecture: {args.arch}
Maintainer: {package['author']['name']} <{package['author']['email']}>
Homepage: {config['help']}
Depends: systemd
Description: Batch file renaming with conflict previews, execution records and undo.
""")
for p in control.iterdir():
    p.chmod(0o644 if p.name == "control" else 0o755)
(control / "conffiles").write_text("")
def archive(directory, excludes=()):
    data = io.BytesIO()
    with tarfile.open(fileobj=data, mode="w:gz", format=tarfile.USTAR_FORMAT) as tar:
        for p in sorted(directory.rglob("*")):
            rel = p.relative_to(directory)
            if rel.parts[0] in excludes:
                continue
            info = tar.gettarinfo(str(p), "./" + str(rel))
            info.uid = info.gid = 0
            info.uname = info.gname = "root"
            with p.open("rb") if p.is_file() else io.BytesIO() as f:
                tar.addfile(info, f if p.is_file() else None)
    return data.getvalue()
out = ROOT / "build" / f"batch-rename_{version}_{args.arch}.deb"
with out.open("wb") as f:
    f.write(b"!<arch>\n")
    for name, content in [("debian-binary", b"2.0\n"), ("control.tar.gz", archive(control)), ("data.tar.gz", archive(stage, ("DEBIAN",)))]:
        header = f"{name+'/':<16}{0:<12}{0:<6}{0:<6}{'100644':<8}{len(content):<10}`\n"
        f.write(header.encode("ascii"))
        f.write(content)
        if len(content) % 2:
            f.write(b"\n")
print(out)
