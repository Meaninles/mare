#!/usr/bin/env python3
import json
import os
import posixpath
import sys
import traceback
import urllib.request
from datetime import datetime, timezone

from p115client import P115Client
from p115client.tool.attr import normalize_attr_web
from p115qrcode import qrcode_result, qrcode_status, qrcode_token, qrcode_url


DEFAULT_APP_TYPE = "windows"
DEFAULT_DOWNLOAD_TIMEOUT_SECONDS = 300
QR_STATUS_LABELS = {
    0: "waiting",
    1: "scanned",
    2: "confirmed",
    -1: "expired",
    -2: "canceled",
    -99: "aborted",
}


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


def should_retry_upload_with_multipart(resp: dict) -> bool:
    if not isinstance(resp, dict):
        return False
    if resp.get("state") is True:
        return False

    message = str(resp.get("message") or "")
    code = str(resp.get("code") or "")
    return code == "10002" or "校验文件失败" in message


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
    return P115Client(cookies=credential, console_qrcode=False, app=resolve_runtime_app_type(app_type))


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


def extract_download_url(value) -> str:
    if isinstance(value, str):
        candidate = value.strip()
        if candidate:
            return candidate

    if isinstance(value, dict):
        for key in ("url", "download_url", "href"):
            candidate = value.get(key)
            if isinstance(candidate, str) and candidate.strip():
                return candidate.strip()

    for attr_name in ("url", "download_url", "href"):
        candidate = getattr(value, attr_name, None)
        if isinstance(candidate, str) and candidate.strip():
            return candidate.strip()

    geturl = getattr(value, "geturl", None)
    if callable(geturl):
        candidate = geturl()
        if isinstance(candidate, str) and candidate.strip():
            return candidate.strip()

    raise ValueError(f"unsupported 115 download url payload: {value!r}")


def extract_download_headers(value) -> dict[str, str]:
    headers = {}

    if isinstance(value, dict):
        candidate = value.get("headers")
        if isinstance(candidate, dict):
            headers.update({
                str(key): str(header_value)
                for key, header_value in candidate.items()
                if header_value is not None
            })
        return headers

    candidate = getattr(value, "headers", None)
    if isinstance(candidate, dict):
        headers.update({
            str(key): str(header_value)
            for key, header_value in candidate.items()
            if header_value is not None
        })

    candidate_dict = getattr(value, "__dict__", None)
    if isinstance(candidate_dict, dict):
        candidate_headers = candidate_dict.get("headers")
        if isinstance(candidate_headers, dict):
            headers.update({
                str(key): str(header_value)
                for key, header_value in candidate_headers.items()
                if header_value is not None
            })

    return headers


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

    try:
        existing = resolve_entry(client, root_id, destination_path)
        if existing and not existing.get("isDir"):
            ensure_state(client.fs_delete(existing["id"]))
    except FileNotFoundError:
        pass

    parent_id = resolve_dir_id(client, root_id, parent_path)
    resp = client.upload_file(
        file=source_file,
        pid=parent_id,
        filename=file_name,
        endpoint="https://oss-cn-shenzhen.aliyuncs.com",
    )
    if should_retry_upload_with_multipart(resp):
        resp = client.upload_file(
            file=source_file,
            pid=parent_id,
            filename=file_name,
            partsize=10 * 1024 * 1024,
            endpoint="https://oss-cn-shenzhen.aliyuncs.com",
        )
    ensure_state(resp)
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
    url_payload = client.download_url(entry["pickcode"], app=resolve_download_app(app_type))
    download_url = extract_download_url(url_payload)
    request_headers = extract_download_headers(url_payload)
    if not any(str(key).lower() == "cookie" for key in request_headers):
        request_headers["Cookie"] = str(client.cookies_str)
    if not any(str(key).lower() == "user-agent" for key in request_headers):
        request_headers["User-Agent"] = "Mozilla/5.0"
    http_request = urllib.request.Request(download_url, headers=request_headers)
    with urllib.request.urlopen(http_request, timeout=resolve_download_timeout_seconds()) as response, open(download_file, "wb") as output:
        output.write(response.read())
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


def main():
    request = json.load(sys.stdin)
    operation = request.get("operation")
    app_type = normalize_app_type(request.get("appType"))

    if operation == "qrcode_start":
        result = {"qrCodeSession": qrcode_start(app_type)}
        json.dump({"success": True, **result}, sys.stdout, ensure_ascii=False)
        return

    if operation == "qrcode_poll":
        result = {"qrCodeSession": qrcode_poll(app_type, request)}
        json.dump({"success": True, **result}, sys.stdout, ensure_ascii=False)
        return

    credential = (request.get("cookies") or request.get("accessToken") or "").strip()
    if not credential:
        raise ValueError("115 session credential is required")

    root_id = int(str(request.get("rootId") or "0"))
    client = build_client(credential, app_type)

    if operation == "health_check":
        fs_list_dir(client, root_id, limit=1, offset=0)
        result = {"healthStatus": "ready", "entry": root_entry(root_id)}
    elif operation == "list_entries":
        result = {"entries": list_entries(client, root_id, request)}
    elif operation == "stat_entry":
        result = {"entry": resolve_entry(client, root_id, request.get("path", ""))}
    elif operation == "copy_in":
        result = {"entry": do_copy_in(client, root_id, app_type, request)}
    elif operation == "copy_out":
        result = do_copy_out(client, root_id, app_type, request)
    elif operation == "delete_entry":
        result = do_delete(client, root_id, request)
    elif operation == "rename_entry":
        result = {"entry": do_rename(client, root_id, request)}
    elif operation == "move_entry":
        result = {"entry": do_move(client, root_id, request)}
    elif operation == "make_directory":
        result = {"entry": do_mkdir(client, root_id, request)}
    else:
        raise ValueError(f"unsupported operation: {operation}")

    json.dump({"success": True, **result}, sys.stdout, ensure_ascii=False)


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        payload = {
            "success": False,
            "error": {
                "message": str(exc),
                "type": exc.__class__.__name__,
                "traceback": traceback.format_exc(),
            },
        }
        json.dump(payload, sys.stdout, ensure_ascii=False)
        sys.exit(1)
