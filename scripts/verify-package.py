#!/usr/bin/env python3
"""Check the packaged TOS window contract and static executable architecture."""
import io
import json
from pathlib import Path
import re
import struct
import sys
import tarfile
import hashlib

ROOT = Path(__file__).resolve().parent.parent
source_config = json.loads((ROOT / "packaging/config.ini").read_text())
source_package = json.loads((ROOT / "package.json").read_text())
if len(sys.argv) < 2:
    sys.exit("Usage: verify-package.py <package.deb> [package.deb ...]")
for arg in sys.argv[1:]:
    p=Path(arg)
    data=p.read_bytes()
    assert data[:8]==b"!<arch>\n", "Not a deb/ar archive"
    members={}
    pos=8
    while pos<len(data):
        header=data[pos:pos+60]
        assert header[-2:]==b"`\n"
        name=header[:16].decode().strip().rstrip("/")
        length=int(header[48:58])
        members[name]=data[pos+60:pos+60+length]
        pos+=60+length+(length%2)
    assert members["debian-binary"]==b"2.0\n"
    with tarfile.open(fileobj=io.BytesIO(members["control.tar.gz"])) as control:
        fields=control.extractfile("./control").read().decode()
        arch=re.search(r"^Architecture: (\S+)$",fields,re.M)[1]
        version=re.search(r"^Version: (\S+)$",fields,re.M)[1]
        assert version == source_package["version"] == source_config["version"], "Package version differs from source"
        assert f"Maintainer: {source_package['author']['name']} <{source_package['author']['email']}>" in fields
        assert f"Homepage: {source_config['help']}" in fields
        for name in ["postinst","prerm","postrm"]:
            assert control.getmember("./"+name).mode & 0o111
            text=control.extractfile("./"+name).read().decode()
            assert "useradd" not in text and "chmod 777" not in text
    with tarfile.open(fileobj=io.BytesIO(members["data.tar.gz"])) as tar:
        names=tar.getnames()
        assert all(n.startswith("./usr") and ".." not in Path(n).parts for n in names)
        base="./usr/local/batch-rename/"
        get=lambda n:tar.extractfile(base+n).read()
        config=json.loads(get("config.ini"))
        expected_config = dict(source_config, platform="x86_64" if arch == "amd64" else "aarch64")
        assert config == expected_config, "Packaged metadata differs from source"
        assert config.get("help"), "Required help URL is missing"
        assert get("batch-rename.lang") == (ROOT / "packaging/batch-rename.lang").read_bytes()
        for name in ["README.md", "THIRD_PARTY_NOTICES.md", "LICENSE"]:
            assert get(name) == (ROOT / name).read_bytes(), f"Packaged {name} differs from source"
        assert config["version"]==version
        assert config["id"]=="batch-rename"
        assert config["type"]=="iframe" and "open_path" not in config
        assert config["path"]=="/batch-rename/"
        assert config["allow_open_in_mobile"] is False, "TNAS Mobile entry must remain disabled pending compatibility verification"
        assert config["resize"] and config["maxmin"] and config["width"]==960 and config["height"]==600
        assert get("images/icons/batch-rename.svg")==(Path(__file__).resolve().parent.parent / "src/assets/rename.svg").read_bytes(), "App and window icons must share the same source"
        assert config["user"]=="batch-rename"
        assert "depend" not in config, "Deb metadata must not declare Docker dependencies"
        assert config["platform"]==("x86_64" if arch=="amd64" else "aarch64")
        binary=get("bin/batch-rename")
        assert binary[:5]==b"\x7fELF\x02"
        info = json.loads(get("build-info.json"))
        assert info == {"version": version, "go": "go" + (ROOT / ".go-version").read_text().strip(),
                        "os": "linux", "arch": arch, "binarySha256": hashlib.sha256(binary).hexdigest()}, "Build information differs from the release toolchain or binary"
        assert version.encode()+b"\x00" in binary, "Backend version literal is missing"
        assert struct.unpack_from("<H",binary,18)[0]==(62 if arch=="amd64" else 183)
        phoff=struct.unpack_from("<Q",binary,32)[0]
        entsize,count=struct.unpack_from("<HH",binary,54)
        assert all(struct.unpack_from("<I",binary,phoff+i*entsize)[0]!=3 for i in range(count)),"ELF unexpectedly needs a dynamic loader"
        assert b"SocketMode=0660" in get("init.d/batch-rename.socket")
        assert b"ListenStream=/var/api/batch-rename.sock" in get("init.d/batch-rename.socket")
        assert b"User=batch-rename" in get("init.d/batch-rename.service")
        service=get("init.d/batch-rename.service").decode()
        assert "EnvironmentFile=" not in service and "-roots" not in service and "-origin" not in service
        assert "Group=batch-rename" in service and "GOMEMLIMIT=192MiB" in service
        assert base + "init.d/batch-rename-setup.service" not in names
        assert "ExecStartPre=+/usr/local/batch-rename/bin/batch-rename -prepare-data" in service
        assert "Restart=" not in service and "RestartSec=" not in service
        assert "StartLimitBurst=3" in service and "StartLimitIntervalSec=60" in service
        assert "ProtectSystem=strict" in service and "CapabilityBoundingSet=\n" in service
        assert b"PartOf=batch-rename.service" in get("init.d/batch-rename.socket")
        assert b"batch-rename-setup" not in get("init.d/batch-rename.socket")
        for unit in ("batch-rename.service", "batch-rename.socket"):
            assert get("init.d/" + unit) == (ROOT / "packaging/init.d" / unit).read_bytes()
        with tarfile.open(fileobj=io.BytesIO(get("webui.bz2")),mode="r:bz2") as web:
            packaged_files = {member.name: web.extractfile(member).read() for member in web if member.isfile()}
            built_files = {str(path.relative_to(ROOT / "dist")): path.read_bytes() for path in (ROOT / "dist").rglob("*") if path.is_file()}
            assert built_files and packaged_files == built_files, "Packaged frontend differs from the current build"
            html=web.extractfile("index.html").read().decode()
            assert 'lang="zh-CN"' in html and 'src="./assets/' in html
            assert f'name="batch-rename-version" content="{version}"' in html, "Frontend build version differs from package"
            assert not re.search(r'(src|href)=[\"\']https?://',html)
            for name in re.findall(r'(?:src|href)="\./([^\"]+)"',html):
                assert web.getmember(name).isfile()
            icon=re.search(r'<link[^>]+rel="icon"[^>]+href="\./([^\"]+)"',html)[1]
            assert web.extractfile(icon).read()==get("images/icons/batch-rename.svg"), "Favicon and packaged icon differ"
            assert any("preview.worker-" in name for name in web.getnames())
        for name in ["licenses/vue.txt","licenses/go.txt","licenses/x-sys.txt"]:
            assert get(name)
        for name in ["COMPATIBILITY.md", "DEVELOPMENT.md", "RELEASE_NOTES.md", "DESIGN.md"]:
            assert get("docs/" + name) == (ROOT / "docs" / name).read_bytes(), f"Packaged {name} differs from source"
    print(f"PASS {p.name}: metadata, {arch} static ELF, socket 0660, current frontend and MIT license")
