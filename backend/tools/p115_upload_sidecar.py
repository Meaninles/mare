#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import traceback
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any
from urllib.parse import urlparse

from cloud115_core import (
    DEFAULT_APP_TYPE,
    DEFAULT_UPLOAD_PART_SIZE,
    abort_upload_session,
    build_client,
    complete_upload_session,
    normalize_app_type,
    normalize_rel_path,
    open_upload_session,
    sanitize_error_payload,
    upload_upload_session_parts,
    list_upload_session_parts,
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Mare p115 upload sidecar")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, required=True)
    parser.add_argument("--state-dir", required=True)
    parser.add_argument("--token", default="")
    parser.add_argument("--mock", action="store_true")
    parser.add_argument("--part-size", type=int, default=DEFAULT_UPLOAD_PART_SIZE)
    return parser.parse_args()


class SidecarError(Exception):
    def __init__(
        self,
        code: str,
        message: str,
        *,
        status_code: int = HTTPStatus.BAD_REQUEST,
        retryable: bool = False,
        detail: Any | None = None,
    ):
        super().__init__(message)
        self.code = (code or "unknown").strip().lower() or "unknown"
        self.message = str(message or "").strip() or "sidecar request failed"
        self.status_code = int(status_code)
        self.retryable = bool(retryable)
        self.detail = detail

    def to_payload(self) -> dict[str, Any]:
        payload: dict[str, Any] = {
            "code": self.code,
            "message": self.message,
            "retryable": self.retryable,
        }
        if self.detail is not None:
            payload["detail"] = self.detail
        return payload


class UploadSidecarService:
    def __init__(self, *, state_dir: str, token: str, mock_mode: bool, part_size: int):
        self.state_dir = Path(state_dir).resolve()
        self.state_dir.mkdir(parents=True, exist_ok=True)
        self.token = (token or "").strip()
        self.mock_mode = bool(mock_mode)
        self.part_size = max(1, int(part_size))
        self.mock_session_dir = self.state_dir.joinpath("mock_sessions")
        self.mock_session_dir.mkdir(parents=True, exist_ok=True)

    def health(self) -> dict[str, Any]:
        return {
            "status": "ready",
            "mode": "mock" if self.mock_mode else "real",
            "pid": os.getpid(),
        }

    def open(self, payload: dict[str, Any]) -> dict[str, Any]:
        if self.mock_mode:
            return self._mock_open(payload)
        app_type = normalize_app_type(payload.get("appType") or DEFAULT_APP_TYPE)
        credential = str(payload.get("credential") or payload.get("cookies") or "").strip()
        if not credential:
            raise SidecarError("auth_required", "credential is required", status_code=HTTPStatus.UNAUTHORIZED)
        root_id = int(str(payload.get("rootId") or "0"))
        client = build_client(credential, app_type)
        request = self._normalize_upload_request(payload)
        return open_upload_session(client, root_id, request)

    def list_parts(self, payload: dict[str, Any]) -> dict[str, Any]:
        if self.mock_mode:
            return self._mock_list(payload)
        app_type = normalize_app_type(payload.get("appType") or DEFAULT_APP_TYPE)
        credential = str(payload.get("credential") or payload.get("cookies") or "").strip()
        if not credential:
            raise SidecarError("auth_required", "credential is required", status_code=HTTPStatus.UNAUTHORIZED)
        client = build_client(credential, app_type)
        request = self._normalize_upload_request(payload)
        return list_upload_session_parts(client, request)

    def upload_parts(self, payload: dict[str, Any]) -> dict[str, Any]:
        if self.mock_mode:
            return self._mock_upload(payload)
        app_type = normalize_app_type(payload.get("appType") or DEFAULT_APP_TYPE)
        credential = str(payload.get("credential") or payload.get("cookies") or "").strip()
        if not credential:
            raise SidecarError("auth_required", "credential is required", status_code=HTTPStatus.UNAUTHORIZED)
        client = build_client(credential, app_type)
        request = self._normalize_upload_request(payload)
        request["maxParts"] = int(payload.get("maxParts") or 0)
        request["partSize"] = int(payload.get("partSize") or self.part_size)
        return upload_upload_session_parts(client, request)

    def complete(self, payload: dict[str, Any]) -> dict[str, Any]:
        if self.mock_mode:
            return self._mock_complete(payload)
        app_type = normalize_app_type(payload.get("appType") or DEFAULT_APP_TYPE)
        credential = str(payload.get("credential") or payload.get("cookies") or "").strip()
        if not credential:
            raise SidecarError("auth_required", "credential is required", status_code=HTTPStatus.UNAUTHORIZED)
        root_id = int(str(payload.get("rootId") or "0"))
        client = build_client(credential, app_type)
        request = self._normalize_upload_request(payload)
        return complete_upload_session(client, root_id, request)

    def abort(self, payload: dict[str, Any]) -> dict[str, Any]:
        if self.mock_mode:
            return self._mock_abort(payload)
        request = self._normalize_upload_request(payload)
        return abort_upload_session(request)

    def _normalize_upload_request(self, payload: dict[str, Any]) -> dict[str, Any]:
        local_path = str(payload.get("localPath") or payload.get("sourceFile") or "").strip()
        destination_path = normalize_rel_path(str(payload.get("remotePath") or payload.get("destinationPath") or ""))
        if not local_path:
            raise SidecarError("invalid_request", "localPath is required")
        if not destination_path:
            raise SidecarError("invalid_request", "remotePath is required")

        job_id = str(payload.get("jobId") or "").strip()
        resume_state_path = str(payload.get("resumeStatePath") or "").strip()
        if not resume_state_path:
            if not job_id:
                raise SidecarError("invalid_request", "jobId is required when resumeStatePath is empty")
            resume_dir = self.state_dir.joinpath("sessions")
            resume_dir.mkdir(parents=True, exist_ok=True)
            resume_state_path = str(resume_dir.joinpath(f"{sanitize_job_id(job_id)}.json"))

        return {
            "sourceFile": os.path.abspath(local_path),
            "destinationPath": destination_path,
            "resumeStatePath": resume_state_path,
            "partSize": int(payload.get("partSize") or self.part_size),
            "parentId": int(payload.get("parentId") or 0),
        }

    def _mock_open(self, payload: dict[str, Any]) -> dict[str, Any]:
        local_path = os.path.abspath(str(payload.get("localPath") or payload.get("sourceFile") or "").strip())
        remote_path = normalize_rel_path(str(payload.get("remotePath") or payload.get("destinationPath") or ""))
        job_id = str(payload.get("jobId") or "").strip()
        if not job_id:
            raise SidecarError("invalid_request", "jobId is required")
        if not local_path or not os.path.exists(local_path):
            raise SidecarError("invalid_request", "localPath is required")
        if not remote_path:
            raise SidecarError("invalid_request", "remotePath is required")

        part_size = max(1, int(payload.get("partSize") or self.part_size))
        file_size = os.path.getsize(local_path)
        total_parts = (file_size + part_size - 1) // part_size if file_size > 0 else 0
        state = self._load_mock_state(job_id)
        if state is None or state.get("localPath") != local_path or state.get("remotePath") != remote_path:
            state = {
                "jobId": job_id,
                "localPath": local_path,
                "remotePath": remote_path,
                "fileSize": int(file_size),
                "partSize": int(part_size),
                "totalParts": int(total_parts),
                "uploadedParts": 0,
            }
        else:
            state["partSize"] = int(part_size)
            state["fileSize"] = int(file_size)
            state["totalParts"] = int(total_parts)
            state["uploadedParts"] = min(int(state.get("uploadedParts") or 0), int(total_parts))
        self._save_mock_state(job_id, state)
        return self._mock_summary(state, uploaded_in_call=[])

    def _mock_list(self, payload: dict[str, Any]) -> dict[str, Any]:
        job_id = str(payload.get("jobId") or "").strip()
        if not job_id:
            raise SidecarError("invalid_request", "jobId is required")
        state = self._load_mock_state(job_id)
        if state is None:
            raise SidecarError("session_not_found", f"mock session not found: {job_id}", status_code=HTTPStatus.NOT_FOUND)
        return self._mock_summary(state, uploaded_in_call=[])

    def _mock_upload(self, payload: dict[str, Any]) -> dict[str, Any]:
        job_id = str(payload.get("jobId") or "").strip()
        if not job_id:
            raise SidecarError("invalid_request", "jobId is required")
        state = self._load_mock_state(job_id)
        if state is None:
            raise SidecarError("session_not_found", f"mock session not found: {job_id}", status_code=HTTPStatus.NOT_FOUND)

        max_parts = max(1, int(payload.get("maxParts") or 1))
        uploaded_parts = int(state.get("uploadedParts") or 0)
        total_parts = int(state.get("totalParts") or 0)
        next_uploaded = min(total_parts, uploaded_parts + max_parts)
        state["uploadedParts"] = next_uploaded
        self._save_mock_state(job_id, state)
        summary = self._mock_summary(state, uploaded_in_call=_mock_part_records(state, uploaded_parts, next_uploaded))
        return summary

    def _mock_complete(self, payload: dict[str, Any]) -> dict[str, Any]:
        job_id = str(payload.get("jobId") or "").strip()
        if not job_id:
            raise SidecarError("invalid_request", "jobId is required")
        state = self._load_mock_state(job_id)
        if state is None:
            raise SidecarError("session_not_found", f"mock session not found: {job_id}", status_code=HTTPStatus.NOT_FOUND)
        summary = self._mock_summary(state, uploaded_in_call=[])
        if not summary["progress"]["completed"]:
            raise SidecarError("incomplete_upload", "upload is not completed", status_code=HTTPStatus.CONFLICT, retryable=True)
        self._delete_mock_state(job_id)
        summary["completed"] = True
        summary["stateDeleted"] = True
        return summary

    def _mock_abort(self, payload: dict[str, Any]) -> dict[str, Any]:
        job_id = str(payload.get("jobId") or "").strip()
        if not job_id:
            raise SidecarError("invalid_request", "jobId is required")
        deleted = self._delete_mock_state(job_id)
        return {"jobId": job_id, "aborted": True, "stateDeleted": deleted}

    def _mock_summary(self, state: dict[str, Any], uploaded_in_call: list[dict[str, Any]]) -> dict[str, Any]:
        parts = _mock_part_records(state, 0, int(state.get("uploadedParts") or 0))
        uploaded_bytes = sum(int(item.get("size") or 0) for item in parts)
        file_size = int(state.get("fileSize") or 0)
        total_parts = int(state.get("totalParts") or 0)
        return {
            "uploadId": f"mock-{state.get('jobId')}",
            "statePath": str(self._mock_path(str(state.get("jobId") or ""))),
            "parts": parts,
            "uploadedInCall": uploaded_in_call,
            "progress": {
                "fileSize": file_size,
                "partSize": int(state.get("partSize") or self.part_size),
                "uploadedBytes": uploaded_bytes,
                "uploadedParts": int(state.get("uploadedParts") or 0),
                "totalParts": total_parts,
                "completed": total_parts > 0 and int(state.get("uploadedParts") or 0) >= total_parts,
            },
            "completed": total_parts > 0 and int(state.get("uploadedParts") or 0) >= total_parts,
        }

    def _mock_path(self, job_id: str) -> Path:
        return self.mock_session_dir.joinpath(f"{sanitize_job_id(job_id)}.json")

    def _load_mock_state(self, job_id: str) -> dict[str, Any] | None:
        path = self._mock_path(job_id)
        if not path.exists():
            return None
        try:
            with path.open("r", encoding="utf-8") as handle:
                payload = json.load(handle)
            return payload if isinstance(payload, dict) else None
        except Exception:
            return None

    def _save_mock_state(self, job_id: str, payload: dict[str, Any]) -> None:
        path = self._mock_path(job_id)
        with path.open("w", encoding="utf-8") as handle:
            json.dump(payload, handle, ensure_ascii=False)

    def _delete_mock_state(self, job_id: str) -> bool:
        path = self._mock_path(job_id)
        try:
            path.unlink()
            return True
        except FileNotFoundError:
            return False


class UploadSidecarServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, address, service: UploadSidecarService):
        super().__init__(address, UploadSidecarHandler)
        self.service = service


class UploadSidecarHandler(BaseHTTPRequestHandler):
    server_version = "MAM-115-Upload-Sidecar/0.1"

    def do_GET(self):
        route = _route(self.path)
        if route == "/healthz":
            self._write_json(HTTPStatus.OK, {"success": True, "data": self.server.service.health()})
            return
        self._write_error(SidecarError("not_found", f"unsupported route: {route}", status_code=HTTPStatus.NOT_FOUND))

    def do_POST(self):
        route = _route(self.path)
        if route != "/healthz":
            auth_error = self._authorize()
            if auth_error is not None:
                self._write_error(auth_error)
                return
        try:
            payload = self._read_payload()
            if route == "/v1/upload/open":
                data = self.server.service.open(payload)
            elif route == "/v1/upload/list-parts":
                data = self.server.service.list_parts(payload)
            elif route == "/v1/upload/upload-parts":
                data = self.server.service.upload_parts(payload)
            elif route == "/v1/upload/complete":
                data = self.server.service.complete(payload)
            elif route == "/v1/upload/abort":
                data = self.server.service.abort(payload)
            else:
                raise SidecarError("not_found", f"unsupported route: {route}", status_code=HTTPStatus.NOT_FOUND)
            self._write_json(HTTPStatus.OK, {"success": True, "data": data})
        except SidecarError as error:
            self._write_error(error)
        except Exception as error:
            payload = sanitize_error_payload(error)
            payload["error"]["traceback"] = traceback.format_exc()
            self._write_json(HTTPStatus.INTERNAL_SERVER_ERROR, payload)

    def log_message(self, format, *args):  # noqa: A003
        pass

    def _authorize(self) -> SidecarError | None:
        token = self.server.service.token
        if not token:
            return None
        provided = (self.headers.get("X-MAM-Sidecar-Token") or "").strip()
        if provided == token:
            return None
        return SidecarError("unauthorized", "invalid sidecar token", status_code=HTTPStatus.UNAUTHORIZED)

    def _read_payload(self) -> dict[str, Any]:
        size = int(self.headers.get("Content-Length", "0") or "0")
        if size <= 0:
            return {}
        raw = self.rfile.read(size)
        if not raw:
            return {}
        try:
            payload = json.loads(raw.decode("utf-8"))
        except Exception as error:
            raise SidecarError("invalid_json", f"invalid JSON payload: {error}") from error
        if not isinstance(payload, dict):
            raise SidecarError("invalid_request", "JSON payload must be object")
        return payload

    def _write_error(self, error: SidecarError):
        self._write_json(max(400, int(error.status_code)), {"success": False, "error": error.to_payload()})

    def _write_json(self, status_code: int, payload: dict[str, Any]):
        encoded = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(int(status_code))
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)


def _route(value: str) -> str:
    route = urlparse(value).path or "/"
    if route != "/":
        route = route.rstrip("/")
    return route


def sanitize_job_id(value: str) -> str:
    value = (value or "").strip()
    if not value:
        return "unknown"
    safe = "".join(char if char.isalnum() or char in {"-", "_", "."} else "_" for char in value)
    return safe.strip("._-") or "unknown"


def _mock_part_records(state: dict[str, Any], start: int, end: int) -> list[dict[str, Any]]:
    file_size = int(state.get("fileSize") or 0)
    part_size = int(state.get("partSize") or DEFAULT_UPLOAD_PART_SIZE)
    start = max(0, int(start))
    end = max(start, int(end))
    parts = []
    for index in range(start, end):
        part_number = index + 1
        begin = index * part_size
        finish = min(file_size, begin + part_size)
        size = max(0, finish - begin)
        parts.append(
            {
                "partNumber": part_number,
                "size": size,
                "etag": f"mock-etag-{part_number}",
            }
        )
    return parts


def main() -> int:
    args = parse_args()
    mock_mode = bool(args.mock or os.getenv("MAM_115_UPLOAD_SIDECAR_MOCK") == "1")
    service = UploadSidecarService(
        state_dir=str(args.state_dir),
        token=str(args.token or ""),
        mock_mode=mock_mode,
        part_size=max(1, int(args.part_size)),
    )
    server = UploadSidecarServer((str(args.host).strip() or "127.0.0.1", int(args.port)), service)
    try:
        server.serve_forever(poll_interval=0.5)
    except KeyboardInterrupt:
        return 0
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
