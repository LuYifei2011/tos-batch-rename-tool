#!/usr/bin/env python3
"""Validate source metadata before building or releasing a package."""
import argparse
import configparser
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parent.parent
LANGUAGES = "zh-cn zh-hk en-us ko-kr ja-jp de-de fr-fr it-it es-es hu-hu pl-pl tr-tr ru-ru pt-pt cs-cz da-dk nl-nl nb-no sv-se el-gr ro-ro th-th vi-vn id-id ar-sa he-il".split()


def validate(root, tag=None):
    package = json.loads((root / "package.json").read_text())
    lock = json.loads((root / "package-lock.json").read_text())
    config = json.loads((root / "packaging/config.ini").read_text())
    version = package["version"]
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version):
        raise ValueError("Version must have three numeric components")
    if {config["version"], lock["version"], lock["packages"][""]["version"]} != {version}:
        raise ValueError("Package, lockfile and TOS versions differ")
    if tag is not None and tag != "v" + version:
        raise ValueError(f"Tag must be v{version}")
    for key in ("id", "icon", "publisher", "exec", "version", "help", "category", "platform", "application_type", "user"):
        if key not in config or config[key] in (None, "", []):
            raise ValueError(f"Missing required config.ini field: {key}")
    if type(config.get("beta")) is not bool or type(config["exec"]) is not bool:
        raise ValueError("beta and exec must be booleans")
    if package["license"] != "MIT" or lock["packages"][""].get("license") != "MIT":
        raise ValueError("Package and lockfile licenses must be MIT")
    author = package["author"]
    if not author.get("email") or config["publisher"] != author["name"]:
        raise ValueError("Publisher must match package author and include a maintainer email")
    if not re.fullmatch(r"https://github\.com/[A-Za-z0-9-]+/[A-Za-z0-9_.-]+", config["help"]):
        raise ValueError("help must point to the GitHub repository")
    if package["repository"]["url"] != config["help"] + ".git" or package["homepage"] != config["help"]:
        raise ValueError("Repository URLs differ")
    license_text = (root / "LICENSE").read_text()
    if not license_text.startswith("MIT License\n") or f"2026 {author['name']}" not in license_text:
        raise ValueError("MIT license copyright does not match the author")
    languages = configparser.ConfigParser(interpolation=None)
    languages.read(root / "packaging/batch-rename.lang", encoding="utf-8")
    if not set(LANGUAGES).issubset(languages.sections()):
        raise ValueError("TOS metadata must include all 26 language sections")
    for lang in languages.sections():
        for key in ("name", "auth", "descript"):
            if not languages[lang].get(key, "").strip().strip('"').strip():
                raise ValueError(f"Missing {lang}.{key}")
        if json.loads(languages[lang]["auth"]) != author["name"]:
            raise ValueError(f"Publisher differs in {lang}")
    return config


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--tag")
    args = parser.parse_args()
    try:
        metadata = validate(ROOT, args.tag)
    except (ValueError, KeyError, OSError) as error:
        parser.exit(1, f"Metadata check failed: {error}\n")
    print(f"PASS metadata: {metadata['version']}, {metadata['publisher']}, MIT")
