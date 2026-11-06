#!/usr/bin/env python3
"""Run browser tests through Vite's proxy against a real backend."""
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
with tempfile.TemporaryDirectory(prefix="mediahub-integration-") as directory:
    binary = Path(directory, "mediahub")
    subprocess.run(["go", "build", "-o", str(binary), "./cmd/mediahub"],
                   cwd=ROOT / "backend", env=dict(os.environ, CGO_ENABLED="1"), check=True)
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    base = f"http://127.0.0.1:{port}"
    env = dict(os.environ, MEDIAHUB_HTTP_ADDR=f"127.0.0.1:{port}",
               MEDIAHUB_DATABASE_PATH=f"{directory}/mediahub.db",
               MEDIAHUB_LOG_FILE="", MEDIAHUB_BACKEND_URL=base,
               MEDIAHUB_E2E_REAL_BACKEND="1", CI="1")
    with tempfile.TemporaryFile(mode="w+") as output:
        process = subprocess.Popen([str(binary)], cwd=directory, env=env,
                                   stdout=output, stderr=output)
        try:
            deadline = time.monotonic() + 30
            while True:
                try:
                    with urllib.request.urlopen(base + "/health/ready", timeout=2) as response:
                        if response.status == 200:
                            break
                except (OSError, urllib.error.URLError):
                    pass
                if time.monotonic() >= deadline:
                    raise RuntimeError("后端未就绪")
                time.sleep(0.1)
            subprocess.run(["pnpm", "test:e2e"], cwd=ROOT / "frontend", env=env, check=True)
        except BaseException:
            output.seek(0)
            print(output.read())
            raise
        finally:
            process.terminate()
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
                raise RuntimeError("后端未能优雅关闭") from None
