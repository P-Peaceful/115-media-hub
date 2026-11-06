#!/usr/bin/env python3
"""Exercise production routing, persistent startup, and graceful shutdown."""
import argparse
import json
import os
import re
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[1]


def response(base, path):
    try:
        with urllib.request.urlopen(base + path, timeout=2) as result:
            return result.status, result.headers, result.read()
    except urllib.error.HTTPError as error:
        return error.code, error.headers, error.read()


def verify(base):
    deadline = time.monotonic() + 30
    while True:
        try:
            if response(base, "/health/ready")[0] == 200:
                break
        except (OSError, urllib.error.URLError):
            pass
        if time.monotonic() >= deadline:
            raise RuntimeError("服务未在 30 秒内就绪")
        time.sleep(0.1)
    for path in ("/health/live", "/health/ready"):
        status, headers, body = response(base, path)
        assert status == 200 and json.loads(body) == {"status": "ok"}, path
        assert headers.get("X-Request-ID"), path
    for path in ("/", "/library/example"):
        status, headers, body = response(base, path)
        assert status == 200 and "text/html" in headers.get("Content-Type", ""), path
        assert b'<div id="root">' in body, path
        for asset in re.findall(rb'(?:src|href)="(/assets/[^"]+)"', body):
            asset_path = asset.decode("utf-8")
            asset_status, asset_headers, asset_body = response(base, asset_path)
            assert asset_status == 200 and asset_body, asset_path
            content_type = asset_headers.get("Content-Type", "")
            assert "text/html" not in content_type, asset_path
            if asset_path.endswith(".css"):
                assert "text/css" in content_type, asset_path
            if asset_path.endswith(".js"):
                assert "javascript" in content_type, asset_path
    for path in ("/api/v1/absent", "/health/absent", "/p/absent", "/assets/absent.js"):
        status, _, body = response(base, path)
        assert status == 404 and b"<!doctype html" not in body.lower(), path
    print("健康检查、SPA 深层路由及 API/健康/播放/静态资源 404 验证通过")


def native():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    with tempfile.TemporaryDirectory(prefix="mediahub-smoke-") as directory:
        env = dict(os.environ, MEDIAHUB_HTTP_ADDR=f"127.0.0.1:{port}",
                   MEDIAHUB_DATABASE_PATH=f"{directory}/mediahub.db",
                   MEDIAHUB_LOG_FILE=f"{directory}/mediahub.log")
        # A second process opens the same database to exercise migration replay.
        for _ in range(2):
            with tempfile.TemporaryFile(mode="w+") as output:
                process = subprocess.Popen([str(ROOT / "bin/mediahub")], cwd=directory,
                                           env=env, stdout=output, stderr=output)
                try:
                    verify(f"http://127.0.0.1:{port}")
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
                        raise RuntimeError("服务未能优雅关闭") from None
                assert code == 0, f"进程退出码：{code}"
        assert Path(directory, "mediahub.db").is_file()
    print("首次及重复启动、SQLite 持久化、SIGTERM 优雅关闭验证通过")


def container(image):
    name = "mediahub-smoke-" + uuid.uuid4().hex[:12]
    try:
        subprocess.run(["docker", "run", "--detach", "--name", name,
                        "--publish", "127.0.0.1::8080", image], check=True)
        binding = subprocess.check_output(["docker", "port", name, "8080/tcp"], text=True).strip()
        verify("http://" + binding)
        subprocess.run(["docker", "stop", "--time", "15", name], check=True)
        code = subprocess.check_output(["docker", "inspect", "--format", "{{.State.ExitCode}}", name], text=True).strip()
        assert code == "0", f"容器退出码：{code}"
        subprocess.run(["docker", "start", name], check=True)
        binding = subprocess.check_output(["docker", "port", name, "8080/tcp"], text=True).strip()
        verify("http://" + binding)
        subprocess.run(["docker", "stop", "--time", "15", name], check=True)
        print("非 root CGO 容器及持久数据重复启动验证通过")
    except BaseException:
        subprocess.run(["docker", "logs", name], check=False)
        raise
    finally:
        subprocess.run(["docker", "rm", "--force", "--volumes", name], check=False)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--docker", metavar="IMAGE")
    args = parser.parse_args()
    if args.docker:
        container(args.docker)
    else:
        native()
