#!/usr/bin/env python3
"""Exercise production routing, persistent startup, and graceful shutdown."""
import argparse
import http.cookiejar
import json
import os
import re
import secrets
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


class AuthenticationSmoke:
    """Keep a real cookie jar across process restarts without exposing secrets."""

    def __init__(self):
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        self.administrator = None
        self.password = secrets.token_urlsafe(24)
        self.previous_password = None

    def bootstrap(self, base):
        with self.opener.open(base + "/api/v1/auth/bootstrap", timeout=5) as result:
            assert result.status == 200
            assert "no-store" in result.headers.get("Cache-Control", "")
            return json.load(result)

    def verify(self, base):
        state = self.bootstrap(base)
        if not state["initialized"]:
            assert self.administrator is None and not state["authenticated"]
            body = json.dumps({
                "username": "smoke-admin",
                "password": self.password,
                "service_url": base,
                "output_directory": "/app/media",
                "session_lifetime_days": 7,
            }).encode("utf-8")
            request = urllib.request.Request(
                base + "/api/v1/auth/initialize", data=body,
                headers={"Content-Type": "application/json",
                         "X-CSRF-Token": state["csrf_token"]}, method="POST")
            with self.opener.open(request, timeout=15) as result:
                assert result.status == 201
                session = json.load(result)
                self.administrator = session["administrator"]["id"]
                cookies = result.headers.get_all("Set-Cookie", [])
                assert any("HttpOnly" in cookie and "SameSite=Lax" in cookie
                           for cookie in cookies)
            state = self.bootstrap(base)
        if self.previous_password is not None:
            assert state["initialized"] and not state["authenticated"]
            try:
                self.login(base, state["csrf_token"], self.previous_password)
            except urllib.error.HTTPError as error:
                assert error.code == 401
                assert json.load(error)["error"]["code"] == "invalid_credentials"
            else:
                raise AssertionError("密码重置后旧密码仍可登录")
            self.login(base, state["csrf_token"], self.password)
            self.previous_password = None
            state = self.bootstrap(base)
        assert state["authenticated"] and state["initialized"]
        assert state["administrator"]["id"] == self.administrator
        assert state["expires_at"] and state["csrf_token"]

    def login(self, base, csrf_token, password):
        request = urllib.request.Request(
            base + "/api/v1/auth/login", method="POST",
            data=json.dumps({"username": "smoke-admin", "password": password,
                             "remember_login": True}).encode("utf-8"),
            headers={"Content-Type": "application/json", "X-CSRF-Token": csrf_token})
        with self.opener.open(request, timeout=15) as result:
            assert result.status == 200
            assert json.load(result)["administrator"]["id"] == self.administrator

    def reset_password(self, command, env=None):
        self.previous_password = self.password
        self.password = secrets.token_urlsafe(24)
        result = subprocess.run(command, env=env,
                                input=(self.password + "\n").encode("utf-8"),
                                capture_output=True, check=True, timeout=30)
        for secret in (self.previous_password, self.password):
            assert secret.encode("utf-8") not in result.stdout + result.stderr


def verify(base, authentication):
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
    authentication.verify(base)
    print("健康检查、SPA/保留路径及真实初始化/登录会话验证通过")


def native():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    with tempfile.TemporaryDirectory(prefix="mediahub-smoke-") as directory:
        env = dict(os.environ, MEDIAHUB_HTTP_ADDR=f"127.0.0.1:{port}",
                   MEDIAHUB_DATABASE_PATH=f"{directory}/mediahub.db",
                   MEDIAHUB_LOG_FILE=f"{directory}/mediahub.log",
                   MEDIAHUB_COOKIE_SECURE="false")
        authentication = AuthenticationSmoke()
        # Reuse cookies across processes to verify persisted session and migrations.
        for launch in range(3):
            if launch == 2:
                authentication.reset_password(
                    [str(ROOT / "bin/mediahub"), "admin", "reset-password",
                     "--password-stdin"], env=env)
            with tempfile.TemporaryFile(mode="w+") as output:
                process = subprocess.Popen([str(ROOT / "bin/mediahub")], cwd=directory,
                                           env=env, stdout=output, stderr=output)
                try:
                    verify(f"http://127.0.0.1:{port}", authentication)
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
    print("首次/重复启动、会话持久化、真实密码恢复与旧会话撤销、SIGTERM 关闭验证通过")


def container(image):
    name = "mediahub-smoke-" + uuid.uuid4().hex[:12]
    authentication = AuthenticationSmoke()
    try:
        subprocess.run(["docker", "run", "--detach", "--name", name,
                        "--publish", "127.0.0.1::8080", image], check=True)
        binding = subprocess.check_output(["docker", "port", name, "8080/tcp"], text=True).strip()
        verify("http://" + binding, authentication)
        subprocess.run(["docker", "stop", "--timeout", "15", name], check=True)
        code = subprocess.check_output(["docker", "inspect", "--format", "{{.State.ExitCode}}", name], text=True).strip()
        assert code == "0", f"容器退出码：{code}"
        subprocess.run(["docker", "start", name], check=True)
        binding = subprocess.check_output(["docker", "port", name, "8080/tcp"], text=True).strip()
        verify("http://" + binding, authentication)
        subprocess.run(["docker", "stop", "--timeout", "15", name], check=True)
        authentication.reset_password(
            ["docker", "run", "--rm", "--interactive", "--volumes-from", name,
             image, "admin", "reset-password", "--password-stdin"])
        subprocess.run(["docker", "start", name], check=True)
        binding = subprocess.check_output(["docker", "port", name, "8080/tcp"], text=True).strip()
        verify("http://" + binding, authentication)
        subprocess.run(["docker", "stop", "--timeout", "15", name], check=True)
        print("非 root CGO 容器、会话持久化、原数据卷密码恢复与旧会话撤销验证通过")
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
