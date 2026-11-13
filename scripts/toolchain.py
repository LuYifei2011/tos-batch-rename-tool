"""Enforce the release Go toolchain before compiling or scanning."""
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parent.parent


def release_go(go="go"):
    expected = "go" + (ROOT / ".go-version").read_text().strip()
    env = dict(os.environ, GOTOOLCHAIN="local")
    actual = subprocess.check_output([go, "env", "GOVERSION"], env=env, text=True).strip()
    if actual != expected:
        raise SystemExit(f"Release builds require {expected}; selected toolchain is {actual}")
    return env, actual


if __name__ == "__main__":
    _, version = release_go()
    print(f"PASS release toolchain: {version}")
