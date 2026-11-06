#!/usr/bin/env python3
"""Fail if either committed transport contract is not reproducible."""
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
FILES = (
    ROOT / "backend/internal/http/generated/types.gen.go",
    ROOT / "frontend/src/lib/api/generated/schema.ts",
)
before = {path: path.read_bytes() for path in FILES}
subprocess.run(["make", "generate"], cwd=ROOT, check=True)
changed = [str(path.relative_to(ROOT)) for path in FILES if path.read_bytes() != before[path]]
if changed:
    raise SystemExit("生成契约发生变化，请重新生成并提交：" + ", ".join(changed))
print("契约重复生成一致")
