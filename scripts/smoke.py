"""Exercise the compiled, embedded application without external services."""
import json
import os
from pathlib import Path
import signal
import socket
import sqlite3
import subprocess
import tempfile
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[1]
with tempfile.TemporaryDirectory(prefix="115-media-hub-smoke-") as directory:
    temporary = Path(directory)
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    env = dict(os.environ, MEDIA_HUB_HTTP_ADDR=f"127.0.0.1:{port}",
               MEDIA_HUB_DATABASE_PATH=str(temporary / "app.db"),
               MEDIA_HUB_LOG_DIR=str(temporary / "logs"))
    with (temporary / "process.log").open("w+") as output:
        process = subprocess.Popen([str(ROOT / "backend/bin/115-media-hub")], env=env,
                                   stdout=output, stderr=subprocess.STDOUT)
        try:
            base = f"http://127.0.0.1:{port}"
            deadline = time.monotonic() + 20
            while True:
                try:
                    with urlopen(base + "/health/ready", timeout=1) as response:
                        assert json.load(response) == {"status": "ok"}
                    break
                except (URLError, TimeoutError):
                    if process.poll() is not None or time.monotonic() >= deadline:
                        output.seek(0)
                        raise RuntimeError(output.read())
                    time.sleep(0.1)
            for path in ["/health/live", "/", "/settings"]:
                with urlopen(Request(base + path, headers={"Accept": "text/html"}), timeout=2) as response:
                    assert response.status == 200, path
                    if path != "/health/live":
                        assert '<div id="root">' in response.read().decode(), path
            for path in ["/api/v1/unknown", "/p/unknown", "/assets/missing.js"]:
                try:
                    urlopen(Request(base + path, headers={"Accept": "text/html"}), timeout=2)
                except HTTPError as error:
                    assert error.code == 404, path
                else:
                    raise AssertionError(f"Expected 404: {path}")
            with sqlite3.connect(temporary / "app.db") as database:
                tables = {row[0] for row in database.execute("SELECT name FROM sqlite_master WHERE type='table'")}
                assert tables <= {"goose_db_version", "sqlite_sequence"}, tables
            process.send_signal(signal.SIGTERM)
            assert process.wait(timeout=15) == 0
            print("PASS: health, embedded SPA, reserved route 404, infrastructure-only schema, graceful shutdown")
        finally:
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
