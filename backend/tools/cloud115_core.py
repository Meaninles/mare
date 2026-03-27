#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import posixpath
import shutil
from datetime import datetime, timezone
from typing import Any, TYPE_CHECKING

if TYPE_CHECKING:
    from p115client import P115Client
    from p115client.tool.upload import P115MultipartUpload


DEFAULT_APP_TYPE = "windows"
DEFAULT_DOWNLOAD_TIMEOUT_SECONDS = 300
DEFAULT_UPLOAD_PART_SIZE = 10 * 1024 * 1024
DEFAULT_DOWNLOAD_USER_AGENT = "Mozilla/5.0"
UPLOAD_SESSION_SUFFIX = ".mam-115-upload.json"
DEFAULT_UPLOAD_ENDPOINT = "https://oss-cn-shenzhen.aliyuncs.com"
QR_STATUS_LABELS = {
    0: "waiting",
    1: "scanned",
    2: "confirmed",
    -1: "expired",
    -2: "canceled",
    -99: "aborted",
}


def _load_p115_client():
    from p115client import P115Client  # type: ignore

    return P115Client


def _load_normalize_attr_web():
    from p115client.tool.attr import normalize_attr_web  # type: ignore

    return normalize_attr_web


def _load_multipart_upload():
    from p115client.tool.upload import P115MultipartUpload  # type: ignore

    return P115MultipartUpload


def _load_qrcode_helpers():
    from p115qrcode import qrcode_result, qrcode_status, qrcode_token, qrcode_url  # type: ignore

    return qrcode_result, qrcode_status, qrcode_token, qrcode_url


def normalize_app_type(value: str) -> str:
    value = (value or "").strip().lower()
    if value in ("", "desktop", "os_windows"):
        return DEFAULT_APP_TYPE
    return value


def resolve_download_timeout_seconds() -> int:
    value = (os.getenv("MAM_115_DOWNLOAD_TIMEOUT_SECONDS") or "").strip()
    if not value:
        return DEFAULT_DOWNLOAD_TIMEOUT_SECONDS
    try:
        parsed = int(value)
        return parsed if parsed > 0 else DEFAULT_DOWNLOAD_TIMEOUT_SECONDS
    except Exception:
        return DEFAULT_DOWNLOAD_TIMEOUT_SECONDS


def resolve_runtime_app_type(value: str) -> str:
    value = normalize_app_type(value)
    if value in {"windows", "mac", "linux", "web", "desktop"}:
        return "web"
    if value in {"android", "115android", "qandroid", "bandroid"}:
        return "android"
    if value in {"ios", "115ios", "qios", "bios"}:
        return "ios"
    if value in {"ipad", "115ipad", "bipad"}:
        return "ipad"
    return value


def normalize_rel_path(path: str) -> str:
    path = (path or "").strip().replace("\\", "/")
    if path in ("", ".", "/"):
        return ""
    normalized = posixpath.normpath("/" + path).lstrip("/")
    return "" if normalized == "." else normalized


def detect_media_type(name: str, is_dir: bool) -> str:
    if is_dir:
        return "unknown"
    ext = posixpath.splitext(name.lower())[1]
    if ext in {".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".heic", ".heif", ".tif", ".tiff", ".raw", ".cr2", ".nef", ".arw"}:
        return "image"
    if ext in {".mp4", ".mov", ".m4v", ".avi", ".mkv", ".wmv", ".flv", ".webm", ".ts", ".m2ts"}:
        return "video"
    if ext in {".mp3", ".wav", ".flac", ".aac", ".m4a", ".ogg", ".wma", ".opus"}:
        return "audio"
    return "unknown"


def timestamp_to_iso(value):
    if value in (None, "", 0):
        return None
    try:
        return datetime.fromtimestamp(int(value), tz=timezone.utc).isoformat().replace("+00:00", "Z")
    except Exception:
        return None


def attr_to_entry(attr: dict, root_id: int, fallback_path: str = "") -> dict:
    is_dir = bool(attr.get("is_dir"))
    path = normalize_rel_path(str(attr.get("path") or fallback_path or ""))
    name = str(attr.get("name") or posixpath.basename(path) or "")
    modified = (
        attr.get("mtime")
        or attr.get("te")
        or attr.get("etime")
        or attr.get("update_time")
        or attr.get("user_utime")
    )
    return {
        "path": path,
        "relativePath": path,
        "name": name,
        "kind": "directory" if is_dir else "file",
        "mediaType": detect_media_type(name, is_dir),
        "size": int(attr.get("size") or 0),
        "modifiedAt": timestamp_to_iso(modified),
        "isDir": is_dir,
        "id": int(attr.get("id") or root_id),
        "parentId": int(attr.get("parent_id") or 0),
    }


def root_entry(root_id: int) -> dict:
    return {
        "path": "",
        "relativePath": "",
        "name": "",
        "kind": "directory",
        "mediaType": "unknown",
        "size": 0,
        "modifiedAt": None,
        "isDir": True,
        "id": root_id,
        "parentId": 0,
    }


def ensure_state(resp: dict):
    if resp.get("state") is True:
        return resp
    raise OSError(str(resp))


def as_cookie_string(cookie_payload) -> str:
    if isinstance(cookie_payload, str):
        return cookie_payload.strip().rstrip(";")
    if isinstance(cookie_payload, dict):
        parts = []
        for key, value in cookie_payload.items():
            parts.append(f"{key}={value}")
        return "; ".join(parts)
    raise ValueError("invalid 115 credential payload")


def build_client(credential: str, app_type: str) -> P115Client:
    client_cls = _load_p115_client()
    return client_cls(cookies=credential, console_qrcode=False, app=resolve_runtime_app_type(app_type))


def fs_list_dir(client: P115Client, dir_id: int, limit: int = 7000, offset: int = 0) -> list[dict]:
    resp = client.fs_files({
        "cid": dir_id,
        "limit": limit,
        "offset": offset,
        "show_dir": 1,
        "count_folders": 1,
        "record_open_time": 1,
        "asc": 1,
        "fc_mix": 1,
        "o": "user_ptime",
        "cur": 1,
        "custom_order": 2,
        "aid": 1,
    })
    ensure_state(resp)
    data = resp.get("data") or []
    normalize_attr_web = _load_normalize_attr_web()
    return [normalize_attr_web(item, simple=True) for item in data]


def resolve_dir_id(client: P115Client, root_id: int, path: str) -> int:
    relative = normalize_rel_path(path)
    if not relative:
        return root_id
    if root_id != 0:
        info = resolve_entry(client, root_id, relative)
        if not info["isDir"]:
            raise NotADirectoryError(relative)
        return int(info["id"])
    resp = client.fs_dir_getid("/" + relative)
    ensure_state(resp)
    cid = resp.get("id") or resp.get("cid") or (resp.get("data") or {}).get("cid") or (resp.get("data") or {}).get("id")
    if cid in (None, "", 0, "0"):
        raise FileNotFoundError(f"path not found: {relative}")
    return int(cid)


def resolve_entry(client: P115Client, root_id: int, path: str) -> dict:
    relative = normalize_rel_path(path)
    if not relative:
        return root_entry(root_id)

    basename = posixpath.basename(relative)
    parent_path = posixpath.dirname(relative)
    parent_id = resolve_dir_id(client, root_id, parent_path)
    for attr in fs_list_dir(client, parent_id):
        if attr["name"] == basename:
            normalized_path = relative
            return {
                "path": normalized_path,
                "relativePath": normalized_path,
                "name": attr["name"],
                "kind": "directory" if attr["is_dir"] else "file",
                "mediaType": detect_media_type(attr["name"], bool(attr["is_dir"])),
                "size": int(attr.get("size") or 0),
                "modifiedAt": timestamp_to_iso(attr.get("mtime")),
                "isDir": bool(attr["is_dir"]),
                "id": int(attr["id"]),
                "parentId": int(attr["parent_id"]),
                "pickcode": attr.get("pickcode") or attr.get("pick_code") or "",
            }
    raise FileNotFoundError(f"path not found: {relative}")


def ensure_dir(client: P115Client, root_id: int, path: str):
    relative = normalize_rel_path(path)
    if not relative:
        return root_entry(root_id)
    current = ""
    current_attr = root_entry(root_id)
    for segment in relative.split("/"):
        next_path = segment if not current else f"{current}/{segment}"
        try:
            current_attr = resolve_entry(client, root_id, next_path)
        except FileNotFoundError:
            parent_attr = resolve_entry(client, root_id, current) if current else root_entry(root_id)
            resp = client.fs_mkdir(segment, pid=parent_attr["id"])
            ensure_state(resp)
            current_attr = resolve_entry(client, root_id, next_path)
        current = next_path
    return current_attr


def list_entries(client: P115Client, root_id: int, request: dict) -> list[dict]:
    target_path = normalize_rel_path(request.get("path", ""))
    target_attr = resolve_entry(client, root_id, target_path)
    if not target_attr.get("isDir"):
        raise ValueError("requested path is not a directory")

    recursive = bool(request.get("recursive"))
    include_directories = bool(request.get("includeDirectories", True))
    media_only = bool(request.get("mediaOnly"))
    limit = int(request.get("limit") or 0)

    queue = [(int(target_attr["id"]), target_path)]
    entries = []
    while queue:
        dir_id, base_path = queue.pop(0)
        for attr in fs_list_dir(client, dir_id):
            entry_path = f"{base_path}/{attr['name']}".strip("/")
            entry = {
                "path": entry_path,
                "relativePath": entry_path,
                "name": attr["name"],
                "kind": "directory" if attr["is_dir"] else "file",
                "mediaType": detect_media_type(attr["name"], bool(attr["is_dir"])),
                "size": int(attr.get("size") or 0),
                "modifiedAt": timestamp_to_iso(attr.get("mtime")),
                "isDir": bool(attr["is_dir"]),
                "id": int(attr["id"]),
                "parentId": int(attr["parent_id"]),
                "pickcode": attr.get("pickcode") or attr.get("pick_code") or "",
            }

            if entry["isDir"]:
                if include_directories:
                    entries.append(entry)
                if recursive:
                    queue.append((int(attr["id"]), entry["path"]))
            else:
                if media_only and entry["mediaType"] == "unknown":
                    continue
                entries.append(entry)

            if limit > 0 and len(entries) >= limit:
                return entries[:limit]
    return entries


def resolve_download_app(app_type: str) -> str:
    app_type = resolve_runtime_app_type(app_type)
    if app_type == "web":
        return "chrome"
    return app_type


def resolve_download_app_candidates(app_type: str) -> list[str]:
    candidates = [
        resolve_download_app(app_type),
        "web",
        "web2",
        "chrome",
        "android",
    ]
    normalized = []
    seen = set()
    for value in candidates:
        value = (value or "").strip()
        if not value or value in seen:
            continue
        seen.add(value)
        normalized.append(value)
    return normalized


def build_upload_headers(client: P115Client) -> dict[str, str]:
    headers = {}
    for key, value in (getattr(client, "headers", None) or {}).items():
        if value is None:
            continue
        headers[str(key)] = str(value)
    return headers


def upload_state_path(source_file: str, resume_state_path: str = "") -> str:
    if (resume_state_path or "").strip():
        return os.path.abspath(resume_state_path)
    return f"{source_file}{UPLOAD_SESSION_SUFFIX}"


def load_upload_state(source_file: str, destination_path: str, parent_id: int, file_name: str, resume_state_path: str = "") -> dict | None:
    state_path = upload_state_path(source_file, resume_state_path)
    if not os.path.exists(state_path):
        return None
    try:
        with open(state_path, "r", encoding="utf-8") as handle:
            payload = json.load(handle)
    except Exception:
        try:
            os.remove(state_path)
        except OSError:
            pass
        return None

    if not isinstance(payload, dict):
        return None

    if normalize_rel_path(str(payload.get("destinationPath") or "")) != destination_path:
        return None
    if parent_id > 0 and int(payload.get("parentId") or 0) != parent_id:
        return None
    if str(payload.get("fileName") or "") != file_name:
        return None
    if not isinstance(payload.get("callback"), dict):
        return None
    if not str(payload.get("url") or "").strip():
        return None
    if not str(payload.get("uploadId") or "").strip():
        return None
    return payload


def save_upload_state(
    source_file: str,
    destination_path: str,
    parent_id: int,
    file_name: str,
    uploader: P115MultipartUpload,
    part_size: int,
    resume_state_path: str = "",
):
    state_path = upload_state_path(source_file, resume_state_path)
    state_dir = os.path.dirname(state_path)
    if state_dir:
        os.makedirs(state_dir, exist_ok=True)
    payload = {
        "destinationPath": destination_path,
        "parentId": parent_id,
        "fileName": file_name,
        "url": uploader.url,
        "callback": uploader.callback,
        "uploadId": uploader.upload_id,
        "partSize": int(part_size),
    }
    with open(state_path, "w", encoding="utf-8") as handle:
        json.dump(payload, handle, ensure_ascii=False)


def clear_upload_state(source_file: str, resume_state_path: str = ""):
    try:
        os.remove(upload_state_path(source_file, resume_state_path))
    except OSError:
        pass


def normalize_uploaded_parts(parts: list[dict]) -> list[dict]:
    normalized = []
    for part in parts or []:
        part_number = int(part.get("PartNumber") or part.get("partNumber") or part.get("part_number") or 0)
        size = int(part.get("Size") or part.get("size") or part.get("partSize") or part.get("part_size") or 0)
        etag = str(part.get("ETag") or part.get("etag") or "").strip()
        value = {
            "partNumber": part_number,
            "size": size,
        }
        if etag:
            value["etag"] = etag
        normalized.append(value)
    normalized.sort(key=lambda item: int(item.get("partNumber") or 0))
    return normalized


def summarize_upload_progress(source_file: str, part_size: int, parts: list[dict]) -> dict:
    file_size = os.path.getsize(source_file)
    uploaded_bytes = sum(int(part.get("size") or 0) for part in parts)
    total_parts = 0
    if file_size > 0 and part_size > 0:
        total_parts = (file_size + part_size - 1) // part_size
    return {
        "fileSize": int(file_size),
        "partSize": int(part_size),
        "uploadedBytes": int(uploaded_bytes),
        "uploadedParts": int(len(parts)),
        "totalParts": int(total_parts),
        "completed": file_size > 0 and uploaded_bytes >= file_size,
    }


def open_upload_session(client: P115Client, root_id: int, request: dict) -> dict:
    destination_path = normalize_rel_path(request.get("destinationPath", ""))
    if not destination_path:
        raise ValueError("destinationPath is required")

    source_file = request.get("sourceFile")
    if not source_file or not os.path.exists(source_file):
        raise ValueError("source file is required")

    requested_part_size = int(request.get("partSize") or DEFAULT_UPLOAD_PART_SIZE)
    if requested_part_size <= 0:
        requested_part_size = DEFAULT_UPLOAD_PART_SIZE

    resume_state_path = str(request.get("resumeStatePath") or "").strip()
    parent_path = posixpath.dirname(destination_path)
    file_name = posixpath.basename(destination_path)
    ensure_dir(client, root_id, parent_path)
    parent_id = resolve_dir_id(client, root_id, parent_path)
    session = load_upload_state(
        source_file,
        destination_path,
        parent_id,
        file_name,
        resume_state_path,
    )

    created = False
    multipart_upload_cls = _load_multipart_upload()
    if session is not None:
        uploader = multipart_upload_cls(session["url"], source_file, session["callback"], str(session["uploadId"]))
        part_size = int(session.get("partSize") or requested_part_size)
    else:
        uploader = multipart_upload_cls.from_path(
            source_file,
            pid=parent_id,
            filename=file_name,
            endpoint=DEFAULT_UPLOAD_ENDPOINT,
            headers=build_upload_headers(client),
        )
        if isinstance(uploader, dict):
            ensure_state(uploader)
            clear_upload_state(source_file, resume_state_path)
            return {
                "sessionCreated": False,
                "sessionExisted": False,
                "uploadId": "",
                "destinationPath": destination_path,
                "statePath": upload_state_path(source_file, resume_state_path),
                "parts": [],
                "progress": summarize_upload_progress(source_file, requested_part_size, []),
                "providerResponse": uploader,
                "completed": True,
            }
        part_size = requested_part_size
        created = True
        save_upload_state(
            source_file,
            destination_path,
            parent_id,
            file_name,
            uploader,
            part_size=part_size,
            resume_state_path=resume_state_path,
        )

    parts = normalize_uploaded_parts(uploader.list_parts())
    progress = summarize_upload_progress(source_file, part_size, parts)
    return {
        "sessionCreated": created,
        "sessionExisted": not created,
        "uploadId": uploader.upload_id,
        "destinationPath": destination_path,
        "statePath": upload_state_path(source_file, resume_state_path),
        "parts": parts,
        "progress": progress,
        "completed": bool(progress["completed"]),
    }


def list_upload_session_parts(client: P115Client, request: dict) -> dict:
    source_file = request.get("sourceFile")
    destination_path = normalize_rel_path(request.get("destinationPath", ""))
    if not source_file or not os.path.exists(source_file):
        raise ValueError("source file is required")
    if not destination_path:
        raise ValueError("destinationPath is required")

    resume_state_path = str(request.get("resumeStatePath") or "").strip()
    parent_id = int(request.get("parentId") or 0)
    file_name = posixpath.basename(destination_path)
    session = load_upload_state(
        source_file,
        destination_path,
        parent_id,
        file_name,
        resume_state_path,
    )
    if session is None:
        raise FileNotFoundError("upload session not found")

    multipart_upload_cls = _load_multipart_upload()
    uploader = multipart_upload_cls(session["url"], source_file, session["callback"], str(session["uploadId"]))
    part_size = int(session.get("partSize") or DEFAULT_UPLOAD_PART_SIZE)
    parts = normalize_uploaded_parts(uploader.list_parts())
    progress = summarize_upload_progress(source_file, part_size, parts)
    return {
        "uploadId": uploader.upload_id,
        "parts": parts,
        "progress": progress,
        "statePath": upload_state_path(source_file, resume_state_path),
    }


def upload_upload_session_parts(client: P115Client, request: dict) -> dict:
    source_file = request.get("sourceFile")
    destination_path = normalize_rel_path(request.get("destinationPath", ""))
    if not source_file or not os.path.exists(source_file):
        raise ValueError("source file is required")
    if not destination_path:
        raise ValueError("destinationPath is required")

    resume_state_path = str(request.get("resumeStatePath") or "").strip()
    parent_id = int(request.get("parentId") or 0)
    file_name = posixpath.basename(destination_path)
    session = load_upload_state(
        source_file,
        destination_path,
        parent_id,
        file_name,
        resume_state_path,
    )
    if session is None:
        raise FileNotFoundError("upload session not found")

    part_size = int(request.get("partSize") or session.get("partSize") or DEFAULT_UPLOAD_PART_SIZE)
    if part_size <= 0:
        part_size = DEFAULT_UPLOAD_PART_SIZE
    max_parts = int(request.get("maxParts") or 0)

    multipart_upload_cls = _load_multipart_upload()
    uploader = multipart_upload_cls(session["url"], source_file, session["callback"], str(session["uploadId"]))
    uploaded_now = []
    iterator = uploader.iter_upload(partsize=part_size)
    try:
        for item in iterator:
            uploaded_now.append(item)
            if max_parts > 0 and len(uploaded_now) >= max_parts:
                break
    finally:
        close = getattr(iterator, "close", None)
        if callable(close):
            close()

    save_upload_state(
        source_file,
        destination_path,
        parent_id,
        file_name,
        uploader,
        part_size=part_size,
        resume_state_path=resume_state_path,
    )

    parts = normalize_uploaded_parts(uploader.list_parts())
    progress = summarize_upload_progress(source_file, part_size, parts)
    return {
        "uploadId": uploader.upload_id,
        "uploadedInCall": normalize_uploaded_parts(uploaded_now),
        "parts": parts,
        "progress": progress,
        "statePath": upload_state_path(source_file, resume_state_path),
    }


def complete_upload_session(client: P115Client, root_id: int, request: dict) -> dict:
    source_file = request.get("sourceFile")
    destination_path = normalize_rel_path(request.get("destinationPath", ""))
    if not source_file or not os.path.exists(source_file):
        raise ValueError("source file is required")
    if not destination_path:
        raise ValueError("destinationPath is required")

    resume_state_path = str(request.get("resumeStatePath") or "").strip()
    parent_id = int(request.get("parentId") or 0)
    file_name = posixpath.basename(destination_path)
    session = load_upload_state(
        source_file,
        destination_path,
        parent_id,
        file_name,
        resume_state_path,
    )
    if session is None:
        raise FileNotFoundError("upload session not found")

    multipart_upload_cls = _load_multipart_upload()
    uploader = multipart_upload_cls(session["url"], source_file, session["callback"], str(session["uploadId"]))
    response = uploader.complete()
    ensure_state(response)
    clear_upload_state(source_file, resume_state_path)
    entry = resolve_entry(client, root_id, destination_path)
    return {
        "uploadId": uploader.upload_id,
        "entry": entry,
        "providerResponse": response,
        "statePath": upload_state_path(source_file, resume_state_path),
        "completed": True,
    }


def abort_upload_session(request: dict) -> dict:
    source_file = request.get("sourceFile")
    if not source_file:
        raise ValueError("source file is required")
    resume_state_path = str(request.get("resumeStatePath") or "").strip()
    state_path = upload_state_path(source_file, resume_state_path)
    existed = os.path.exists(state_path)
    clear_upload_state(source_file, resume_state_path)
    return {
        "statePath": state_path,
        "stateDeleted": existed,
        "aborted": True,
    }


def do_copy_in(client: P115Client, root_id: int, app_type: str, request: dict) -> dict:
    destination_path = normalize_rel_path(request.get("destinationPath", ""))
    if not destination_path:
        raise ValueError("destinationPath is required")

    source_file = request.get("sourceFile")
    if not source_file or not os.path.exists(source_file):
        raise ValueError("source file is required")

    parent_path = posixpath.dirname(destination_path)
    file_name = posixpath.basename(destination_path)
    ensure_dir(client, root_id, parent_path)
    parent_id = resolve_dir_id(client, root_id, parent_path)
    resume_state_path = str(request.get("resumeStatePath") or "").strip()
    session = load_upload_state(source_file, destination_path, parent_id, file_name, resume_state_path)

    if session is None:
        try:
            existing = resolve_entry(client, root_id, destination_path)
            if existing and not existing.get("isDir"):
                ensure_state(client.fs_delete(existing["id"]))
        except FileNotFoundError:
            pass

    open_upload_session(
        client,
        root_id,
        {
            **request,
            "destinationPath": destination_path,
            "sourceFile": source_file,
            "resumeStatePath": resume_state_path,
            "partSize": int(request.get("partSize") or DEFAULT_UPLOAD_PART_SIZE),
        },
    )

    upload_upload_session_parts(
        client,
        {
            "sourceFile": source_file,
            "destinationPath": destination_path,
            "parentId": parent_id,
            "resumeStatePath": resume_state_path,
            "partSize": int(request.get("partSize") or DEFAULT_UPLOAD_PART_SIZE),
            "maxParts": 0,
        },
    )
    complete_upload_session(
        client,
        root_id,
        {
            "sourceFile": source_file,
            "destinationPath": destination_path,
            "parentId": parent_id,
            "resumeStatePath": resume_state_path,
        },
    )
    return resolve_entry(client, root_id, destination_path)


def do_copy_out(client: P115Client, root_id: int, app_type: str, request: dict) -> dict:
    source_path = normalize_rel_path(request.get("path", ""))
    if not source_path:
        raise ValueError("path is required")
    download_file = request.get("downloadFile")
    if not download_file:
        raise ValueError("download file path is required")
    entry = resolve_entry(client, root_id, source_path)
    if entry.get("isDir"):
        raise IsADirectoryError(source_path)
    last_error = None
    for candidate_app in resolve_download_app_candidates(app_type):
        try:
            url_payload = client.download_url(
                entry["pickcode"],
                app=candidate_app,
                user_agent=DEFAULT_DOWNLOAD_USER_AGENT,
            )
            stream = client.open(
                url_payload,
                headers={"User-Agent": DEFAULT_DOWNLOAD_USER_AGENT},
            )
            try:
                with open(download_file, "wb") as output:
                    shutil.copyfileobj(stream, output, length=1024 * 1024)
            finally:
                stream.close()
            return {"downloadFile": download_file}
        except Exception as exc:
            last_error = exc
    if last_error is not None:
        raise last_error
    return {"downloadFile": download_file}


def do_delete(client: P115Client, root_id: int, request: dict) -> dict:
    source_path = normalize_rel_path(request.get("path", ""))
    if not source_path:
        raise ValueError("path is required")
    entry = resolve_entry(client, root_id, source_path)
    ensure_state(client.fs_delete(entry["id"]))
    return {}


def do_rename(client: P115Client, root_id: int, request: dict) -> dict:
    source_path = normalize_rel_path(request.get("path", ""))
    new_name = (request.get("newName") or "").strip()
    if not source_path:
        raise ValueError("path is required")
    if not new_name:
        raise ValueError("newName is required")
    entry = resolve_entry(client, root_id, source_path)
    ensure_state(client.fs_rename((entry["id"], new_name)))
    parent_path = posixpath.dirname(source_path)
    final_path = f"{parent_path}/{new_name}".strip("/")
    return resolve_entry(client, root_id, final_path)


def do_move(client: P115Client, root_id: int, request: dict) -> dict:
    source_path = normalize_rel_path(request.get("path", ""))
    destination_path = normalize_rel_path(request.get("destinationPath", ""))
    if not source_path:
        raise ValueError("path is required")
    if not destination_path:
        raise ValueError("destinationPath is required")

    destination_parent = posixpath.dirname(destination_path)
    destination_name = posixpath.basename(destination_path)
    source_name = posixpath.basename(source_path)

    source_entry = resolve_entry(client, root_id, source_path)
    destination_parent_attr = ensure_dir(client, root_id, destination_parent)
    ensure_state(client.fs_move(source_entry["id"], pid=destination_parent_attr["id"]))

    final_path = destination_path
    if source_name != destination_name:
        moved_path = f"{destination_parent}/{source_name}".strip("/")
        moved_entry = resolve_entry(client, root_id, moved_path)
        ensure_state(client.fs_rename((moved_entry["id"], destination_name)))

    return resolve_entry(client, root_id, final_path)


def do_mkdir(client: P115Client, root_id: int, request: dict) -> dict:
    path = normalize_rel_path(request.get("path", ""))
    if not path:
        raise ValueError("path is required")
    return ensure_dir(client, root_id, path)


def qrcode_start(app_type: str) -> dict:
    _, _, qrcode_token, qrcode_url = _load_qrcode_helpers()
    token = qrcode_token()
    return {
        "uid": token["uid"],
        "time": int(token["time"]),
        "sign": token["sign"],
        "appType": app_type,
        "qrCodeUrl": qrcode_url(token["uid"]),
        "status": "waiting",
        "statusCode": 0,
    }


def qrcode_poll(app_type: str, request: dict) -> dict:
    qrcode_result, qrcode_status, _, qrcode_url = _load_qrcode_helpers()
    uid = (request.get("qrUid") or "").strip()
    qr_sign = (request.get("qrSign") or "").strip()
    qr_time = int(request.get("qrTime") or 0)
    if not uid or not qr_sign or not qr_time:
        raise ValueError("qr token payload is required")

    token = {"uid": uid, "time": qr_time, "sign": qr_sign}
    status_resp = qrcode_status(token)
    status_code = int(status_resp.get("status", -99))
    payload = {
        "uid": uid,
        "time": qr_time,
        "sign": qr_sign,
        "appType": app_type,
        "qrCodeUrl": qrcode_url(uid),
        "status": QR_STATUS_LABELS.get(status_code, "unknown"),
        "statusCode": status_code,
    }
    if status_code == 2:
        result = qrcode_result(uid, app_type)
        credential = as_cookie_string(result.get("cookie") or result.get("cookies") or {})
        payload["credential"] = credential
    return payload


def dispatch_bridge_operation(request: dict) -> dict:
    operation = request.get("operation")
    app_type = normalize_app_type(request.get("appType"))

    if operation == "qrcode_start":
        return {"success": True, "qrCodeSession": qrcode_start(app_type)}

    if operation == "qrcode_poll":
        return {"success": True, "qrCodeSession": qrcode_poll(app_type, request)}

    credential = (request.get("cookies") or request.get("accessToken") or "").strip()
    if not credential:
        raise ValueError("115 session credential is required")

    root_id = int(str(request.get("rootId") or "0"))
    client = build_client(credential, app_type)

    if operation == "health_check":
        fs_list_dir(client, root_id, limit=1, offset=0)
        return {"success": True, "healthStatus": "ready", "entry": root_entry(root_id)}

    if operation == "list_entries":
        return {"success": True, "entries": list_entries(client, root_id, request)}
    if operation == "stat_entry":
        return {"success": True, "entry": resolve_entry(client, root_id, request.get("path", ""))}
    if operation == "copy_in":
        return {"success": True, "entry": do_copy_in(client, root_id, app_type, request)}
    if operation == "copy_out":
        return {"success": True, **do_copy_out(client, root_id, app_type, request)}
    if operation == "delete_entry":
        return {"success": True, **do_delete(client, root_id, request)}
    if operation == "rename_entry":
        return {"success": True, "entry": do_rename(client, root_id, request)}
    if operation == "move_entry":
        return {"success": True, "entry": do_move(client, root_id, request)}
    if operation == "make_directory":
        return {"success": True, "entry": do_mkdir(client, root_id, request)}
    if operation == "upload_session_open":
        return {"success": True, "uploadSession": open_upload_session(client, root_id, request)}
    if operation == "upload_session_list_parts":
        return {"success": True, "uploadSession": list_upload_session_parts(client, request)}
    if operation == "upload_session_upload_parts":
        return {"success": True, "uploadSession": upload_upload_session_parts(client, request)}
    if operation == "upload_session_complete":
        return {"success": True, "uploadSession": complete_upload_session(client, root_id, request)}
    if operation == "upload_session_abort":
        return {"success": True, "uploadSession": abort_upload_session(request)}

    raise ValueError(f"unsupported operation: {operation}")


def sanitize_error_payload(error: Exception) -> dict[str, Any]:
    return {
        "success": False,
        "error": {
            "message": str(error),
            "type": error.__class__.__name__,
        },
    }
