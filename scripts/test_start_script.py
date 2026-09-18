"""Exercise start.sh in an isolated checkout with local fixture servers only."""
import http.server
import os
from pathlib import Path
import shlex
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time
import unittest


def fixture(role, serve=False):
    port = int(os.environ["BACKEND_PORT" if role == "backend" else "FRONTEND_PORT"])
    mode = os.environ.get(role.upper() + "_MODE", "ready")
    if not serve:
        Path(role + ".started").touch()
        if mode == "fail":
            sys.exit(2)
        child = subprocess.Popen([sys.executable, __file__, "--serve", role], stdout=subprocess.PIPE, text=True)
        Path(role + ".pid").write_text(str(child.pid))
        if child.stdout.readline().strip() != "ready":
            sys.exit(3)
        if mode == "orphan":
            sys.exit(4)
        sys.exit(child.wait())

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'{"success":false}' if mode == "unhealthy" else b'{"success":true}')

        def log_message(self, *args):
            pass

    with http.server.HTTPServer(("127.0.0.1", port), Handler) as server:
        print("ready", flush=True)
        server.serve_forever()


class StartScriptTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="yuan-start-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        source = Path(__file__).resolve().parents[1] / "start.sh"
        shutil.copy2(source, self.root / "start.sh")
        (self.root / "bin").mkdir()
        (self.root / "web/node_modules/.bin").mkdir(parents=True)
        (self.root / "web/dist").mkdir()
        (self.root / "web/dist/index.html").touch()
        stub = self.root / "web/node_modules/.bin/rsbuild"
        stub.touch()
        stub.chmod(0o700)
        for command, role in [("go", "backend"), ("bun", "frontend")]:
            wrapper = self.root / "bin" / command
            wrapper.write_text("#!/bin/sh\nexec " + shlex.join([sys.executable, str(Path(__file__).resolve()), "--fixture", role]) + "\n")
            wrapper.chmod(0o700)
        with socket.socket() as first, socket.socket() as second:
            first.bind(("127.0.0.1", 0))
            second.bind(("127.0.0.1", 0))
            self.ports = [first.getsockname()[1], second.getsockname()[1]]
        self.env = dict(os.environ, PATH=str(self.root / "bin") + os.pathsep + os.environ["PATH"],
                        BACKEND_PORT=str(self.ports[0]), FRONTEND_PORT=str(self.ports[1]), STARTUP_TIMEOUT="3")

    def launch(self, **modes):
        log = open(self.root / "output", "w+")
        self.addCleanup(log.close)
        proc = subprocess.Popen(["bash", "start.sh"], cwd=self.root, env=dict(self.env, **modes), stdout=log, stderr=log)
        self.addCleanup(self.stop, proc)
        return proc

    def stop(self, proc):
        if proc.poll() is None:
            proc.send_signal(signal.SIGTERM)
            proc.wait(timeout=15)

    def wait_ready(self, proc):
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            output = (self.root / "output").read_text()
            if "持续监控前后端日志" in output:
                return
            self.assertIsNone(proc.poll(), output)
            time.sleep(0.05)
        self.fail("startup readiness timed out")

    def assert_released(self):
        for port in self.ports:
            with socket.socket() as connection:
                self.assertNotEqual(connection.connect_ex(("127.0.0.1", port)), 0)
        for pid_file in [self.root / "backend.pid", self.root / "web/frontend.pid"]:
            if pid_file.exists():
                state = subprocess.run(["ps", "-p", pid_file.read_text(), "-o", "stat="], capture_output=True, text=True).stdout.strip()
                self.assertTrue(not state or state.startswith("Z"), f"fixture child survived: {state}")

    def test_backend_failure_does_not_launch_frontend(self):
        proc = self.launch(BACKEND_MODE="fail")
        self.assertNotEqual(proc.wait(timeout=15), 0)
        self.assertFalse((self.root / "web/frontend.started").exists())
        self.assert_released()

    def test_listening_but_unhealthy_backend_is_not_ready(self):
        proc = self.launch(BACKEND_MODE="unhealthy")
        self.assertNotEqual(proc.wait(timeout=15), 0)
        self.assertFalse((self.root / "web/frontend.started").exists())
        self.assert_released()

    def test_frontend_build_failure_cleans_backend(self):
        proc = self.launch(FRONTEND_MODE="fail")
        self.assertNotEqual(proc.wait(timeout=15), 0)
        self.assert_released()

    def test_frontend_parent_failure_cleans_orphan_and_backend(self):
        proc = self.launch(FRONTEND_MODE="orphan")
        self.assertNotEqual(proc.wait(timeout=15), 0)
        self.assert_released()

    def test_signals_release_children_and_allow_restart(self):
        for sig in [signal.SIGINT, signal.SIGTERM]:
            proc = self.launch()
            self.wait_ready(proc)
            proc.send_signal(sig)
            self.assertEqual(proc.wait(timeout=15), 128 + sig)
            self.assert_released()

    def test_running_service_exit_stops_other_service(self):
        proc = self.launch()
        self.wait_ready(proc)
        os.kill(int((self.root / "backend.pid").read_text()), signal.SIGTERM)
        self.assertNotEqual(proc.wait(timeout=15), 0)
        self.assert_released()

    def occupy_backend_port(self):
        # The listener must be a disposable child, never the test runner itself:
        # start.sh is explicitly expected to terminate existing listeners.
        child = subprocess.Popen([sys.executable, __file__, "--serve", "backend"],
                                 env=self.env, stdout=subprocess.PIPE, text=True)
        self.addCleanup(child.stdout.close)
        self.addCleanup(self.stop, child)
        self.assertEqual(child.stdout.readline().strip(), "ready")
        return child

    def test_occupied_port_is_stopped_then_restarted(self):
        child = self.occupy_backend_port()
        proc = self.launch()
        self.wait_ready(proc)
        self.assertEqual(child.wait(timeout=5), -signal.SIGTERM)
        self.stop(proc)
        self.assert_released()

    def test_invalid_configuration_preserves_existing_process(self):
        for config in [{"FRONTEND_PORT": "invalid"},
                       {"FRONTEND_PORT": self.env["BACKEND_PORT"]},
                       {"STARTUP_TIMEOUT": "invalid"},
                       {"STARTUP_TIMEOUT": "999999999999999999999"}]:
            with self.subTest(config=config):
                child = self.occupy_backend_port()
                proc = self.launch(**config)
                self.assertNotEqual(proc.wait(timeout=15), 0)
                self.assertFalse((self.root / "backend.started").exists())
                self.assertIsNone(child.poll(), "invalid parameters must not stop the running service")
                self.stop(child)


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] in ("--fixture", "--serve"):
        fixture(sys.argv[2], sys.argv[1] == "--serve")
    else:
        unittest.main()
