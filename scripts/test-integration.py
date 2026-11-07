#!/usr/bin/env python3
"""Run each browser project against its own real backend and fresh database."""
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
# Keep these aligned with frontend/playwright.config.ts. Each project's single
# real-auth scenario owns initialization; mocked scenarios remain independent.
BROWSER_PROJECTS = ("desktop", "mobile-dark")


def available_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def wait_ready(base, process):
    deadline = time.monotonic() + 30
    while process.poll() is None:
        try:
            with urllib.request.urlopen(base + "/health/ready", timeout=2) as response:
                if response.status == 200:
                    return
        except (OSError, urllib.error.URLError):
            pass
        if time.monotonic() >= deadline:
            raise RuntimeError("后端未在 30 秒内就绪")
        time.sleep(0.1)
    raise RuntimeError(f"后端提前退出：{process.returncode}")


def run_project(binary, project):
    with tempfile.TemporaryDirectory(prefix=f"mediahub-{project}-") as directory:
        port = available_port()
        base = f"http://127.0.0.1:{port}"
        env = dict(os.environ, MEDIAHUB_HTTP_ADDR=f"127.0.0.1:{port}",
                   MEDIAHUB_DATABASE_PATH=f"{directory}/mediahub.db",
                   MEDIAHUB_LOG_FILE="", MEDIAHUB_COOKIE_SECURE="false",
                   MEDIAHUB_BACKEND_URL=base, MEDIAHUB_E2E_REAL_BACKEND="1", CI="1")
        with tempfile.TemporaryFile(mode="w+") as output:
            process = subprocess.Popen([str(binary)], cwd=directory, env=env,
                                       stdout=output, stderr=output)
            try:
                wait_ready(base, process)
                subprocess.run(["pnpm", "test:e2e", f"--project={project}"],
                               cwd=ROOT / "frontend", env=env, check=True)
            except BaseException:
                output.seek(0)
                print(output.read())
                raise
            finally:
                process.terminate()
                try:
                    code = process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
                    raise RuntimeError("后端未能优雅关闭") from None
                if code != 0:
                    raise RuntimeError(f"后端退出码：{code}")


def main():
    with tempfile.TemporaryDirectory(prefix="mediahub-integration-build-") as directory:
        binary = Path(directory, "mediahub")
        subprocess.run(["go", "build", "-o", str(binary), "./cmd/mediahub"],
                       cwd=ROOT / "backend", env=dict(os.environ, CGO_ENABLED="1"), check=True)
        for project in BROWSER_PROJECTS:
            run_project(binary, project)


if __name__ == "__main__":
    main()
