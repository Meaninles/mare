import json
import os
import socket
import subprocess
import tempfile
import time
import unittest
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import ProxyHandler, Request, build_opener


def allocate_port() -> int:
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()
    return int(port)


class P115UploadSidecarMockTests(unittest.TestCase):
    def setUp(self):
        self.workspace = Path(__file__).resolve().parents[3]
        self.sidecar_path = self.workspace.joinpath("backend", "tools", "p115_upload_sidecar.py")
        if not self.sidecar_path.exists():
            self.fail(f"sidecar script not found: {self.sidecar_path}")

        self.token = "test-sidecar-token"
        self.port = allocate_port()
        self.state_dir = tempfile.TemporaryDirectory(prefix="mam-p115-sidecar-tests-")
        self.temp_dir = tempfile.TemporaryDirectory(prefix="mam-p115-sidecar-files-")
        self.local_file = Path(self.temp_dir.name).joinpath("demo.bin")
        self.local_file.write_bytes(os.urandom(4096))

        python_cmd = os.getenv("MAM_PYTHON_CMD", "python")
        env = os.environ.copy()
        env["PYTHONIOENCODING"] = "utf-8"
        env["MAM_115_UPLOAD_SIDECAR_MOCK"] = "1"
        env["NO_PROXY"] = "127.0.0.1,localhost"
        self.process = subprocess.Popen(
            [
                python_cmd,
                str(self.sidecar_path),
                "--host",
                "127.0.0.1",
                "--port",
                str(self.port),
                "--state-dir",
                self.state_dir.name,
                "--token",
                self.token,
                "--mock",
                "--part-size",
                "1024",
            ],
            cwd=str(self.workspace),
            env=env,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        self.http = build_opener(ProxyHandler({}))
        self._wait_for_ready()

    def tearDown(self):
        if getattr(self, "process", None) is not None:
            self.process.terminate()
            try:
                self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
            if self.process.stdout:
                self.process.stdout.close()
            if self.process.stderr:
                self.process.stderr.close()
        if getattr(self, "state_dir", None):
            self.state_dir.cleanup()
        if getattr(self, "temp_dir", None):
            self.temp_dir.cleanup()

    def test_upload_session_lifecycle_in_mock_mode(self):
        open_response = self._post(
            "/v1/upload/open",
            {
                "jobId": "job-sidecar-1",
                "localPath": str(self.local_file),
                "remotePath": "/测试目录/示例.bin",
                "partSize": 1024,
            },
        )
        self.assertTrue(open_response["success"])
        self.assertEqual(open_response["data"]["progress"]["totalParts"], 4)

        upload_response = self._post(
            "/v1/upload/upload-parts",
            {
                "jobId": "job-sidecar-1",
                "localPath": str(self.local_file),
                "remotePath": "/测试目录/示例.bin",
                "maxParts": 2,
            },
        )
        self.assertEqual(upload_response["data"]["progress"]["uploadedParts"], 2)
        self.assertEqual(len(upload_response["data"]["uploadedInCall"]), 2)

        list_response = self._post(
            "/v1/upload/list-parts",
            {
                "jobId": "job-sidecar-1",
                "localPath": str(self.local_file),
                "remotePath": "/测试目录/示例.bin",
            },
        )
        self.assertEqual(list_response["data"]["progress"]["uploadedParts"], 2)

        upload_finish = self._post(
            "/v1/upload/upload-parts",
            {
                "jobId": "job-sidecar-1",
                "localPath": str(self.local_file),
                "remotePath": "/测试目录/示例.bin",
                "maxParts": 8,
            },
        )
        self.assertTrue(upload_finish["data"]["progress"]["completed"])

        complete_response = self._post(
            "/v1/upload/complete",
            {
                "jobId": "job-sidecar-1",
                "localPath": str(self.local_file),
                "remotePath": "/测试目录/示例.bin",
            },
        )
        self.assertTrue(complete_response["data"]["completed"])
        self.assertTrue(complete_response["data"]["stateDeleted"])

        abort_response = self._post(
            "/v1/upload/abort",
            {
                "jobId": "job-sidecar-1",
                "localPath": str(self.local_file),
                "remotePath": "/测试目录/示例.bin",
            },
        )
        self.assertTrue(abort_response["data"]["aborted"])

    def test_upload_session_persists_between_requests(self):
        self._post(
            "/v1/upload/open",
            {
                "jobId": "job-sidecar-2",
                "localPath": str(self.local_file),
                "remotePath": "/测试目录/恢复.bin",
                "partSize": 1024,
            },
        )
        self._post(
            "/v1/upload/upload-parts",
            {
                "jobId": "job-sidecar-2",
                "localPath": str(self.local_file),
                "remotePath": "/测试目录/恢复.bin",
                "maxParts": 1,
            },
        )

        list_response = self._post(
            "/v1/upload/list-parts",
            {
                "jobId": "job-sidecar-2",
                "localPath": str(self.local_file),
                "remotePath": "/测试目录/恢复.bin",
            },
        )
        self.assertEqual(list_response["data"]["progress"]["uploadedParts"], 1)

    def _wait_for_ready(self):
        deadline = time.time() + 10
        last_error = None
        while time.time() < deadline:
            if self.process.poll() is not None:
                stdout = (self.process.stdout.read() if self.process.stdout else "") or ""
                stderr = (self.process.stderr.read() if self.process.stderr else "") or ""
                self.fail(f"sidecar exited early: code={self.process.returncode} stdout={stdout} stderr={stderr}")
            try:
                result = self._get("/healthz", authorized=False)
                if result.get("success"):
                    return
            except Exception as error:  # noqa: BLE001
                last_error = error
            time.sleep(0.1)
        self.fail(f"sidecar did not become ready in time: {last_error}")

    def _get(self, path: str, authorized: bool = True):
        request = Request(f"http://127.0.0.1:{self.port}{path}", method="GET")
        if authorized:
            request.add_header("X-MAM-Sidecar-Token", self.token)
        with self.http.open(request, timeout=5) as response:
            return json.loads(response.read().decode("utf-8"))

    def _post(self, path: str, payload: dict):
        request = Request(
            f"http://127.0.0.1:{self.port}{path}",
            data=json.dumps(payload).encode("utf-8"),
            method="POST",
            headers={
                "Content-Type": "application/json",
                "X-MAM-Sidecar-Token": self.token,
            },
        )
        try:
            with self.http.open(request, timeout=10) as response:
                return json.loads(response.read().decode("utf-8"))
        except HTTPError as error:
            detail = error.read().decode("utf-8", errors="replace")
            self.fail(f"request failed: {error.code} {detail}")


if __name__ == "__main__":
    unittest.main()
