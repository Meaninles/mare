import json
import math
import os
import subprocess
import sys
import tempfile
from pathlib import Path


_CLIP_CACHE = {}
_WHISPER_CACHE = {}


def main() -> int:
    try:
        request = json.loads(sys.stdin.read() or "{}")
        operation = str(request.get("operation", "")).strip().lower()

        if operation == "transcribe":
            payload = transcribe_media(
                input_path=str(request.get("inputPath", "")).strip(),
                ffmpeg_path=str(request.get("ffmpegPath", "")).strip(),
            )
            return write_response({"success": True, "transcript": payload})

        if operation == "embed_text":
            payload = embed_text(str(request.get("text", "")).strip())
            return write_response({"success": True, "embedding": payload})

        if operation == "embed_image":
            payload = embed_image(str(request.get("inputPath", "")).strip())
            return write_response({"success": True, "embedding": payload})

        if operation == "embed_video":
            payload = embed_video(
                input_path=str(request.get("inputPath", "")).strip(),
                ffmpeg_path=str(request.get("ffmpegPath", "")).strip(),
            )
            return write_response({"success": True, "embedding": payload})

        if operation == "describe_vector":
            payload = describe_vector(
                vector_input=request.get("vector"),
                label_entries=request.get("labels"),
                top_k=request.get("topK"),
            )
            return write_response({"success": True, "description": payload})

        raise ValueError(f"unsupported operation: {operation}")
    except Exception as exc:
        return write_response(
            {
                "success": False,
                "error": {
                    "message": str(exc),
                    "type": exc.__class__.__name__,
                },
            }
        )


def write_response(payload: dict) -> int:
    sys.stdout.write(json.dumps(payload, ensure_ascii=False))
    return 0 if payload.get("success") else 1


def transcribe_media(input_path: str, ffmpeg_path: str) -> dict:
    ensure_existing_file(input_path)
    prepare_ffmpeg_path(ffmpeg_path)

    model_name = os.getenv("MAM_SEARCH_WHISPER_MODEL", "tiny")
    compute_type = os.getenv("MAM_SEARCH_WHISPER_COMPUTE", "int8")

    cache_key = (model_name, compute_type)
    model = _WHISPER_CACHE.get(cache_key)
    if model is None:
        try:
            from faster_whisper import WhisperModel
        except ImportError as exc:
            raise RuntimeError(
                "缺少 faster-whisper 依赖，请安装 backend/tools/search_ai.requirements.txt"
            ) from exc

        model = WhisperModel(model_name, device="cpu", compute_type=compute_type)
        _WHISPER_CACHE[cache_key] = model

    try:
        segments, info = model.transcribe(input_path, beam_size=1, vad_filter=True)
    except IndexError as exc:
        raise RuntimeError("媒体音轨无法被 faster-whisper 正常读取，请确认文件包含可解码音轨") from exc
    text_parts = []
    for segment in segments:
        value = str(getattr(segment, "text", "")).strip()
        if value:
            text_parts.append(value)

    transcript_text = " ".join(text_parts).strip()
    if not transcript_text:
        return {
            "text": "",
            "language": getattr(info, "language", "") or "",
            "modelName": model_name,
        }

    if not transcript_text:
        raise RuntimeError("未生成可用转写文本")

    return {
        "text": transcript_text,
        "language": getattr(info, "language", "") or "",
        "modelName": model_name,
    }


def embed_text(text: str) -> dict:
    if not text:
        raise ValueError("text is required")

    model, processor, model_name, torch = get_clip_bundle()
    with torch.no_grad():
        inputs = processor(text=[text], return_tensors="pt", padding=True, truncation=True)
        features = resolve_feature_rows(model.get_text_features(**inputs))

    vector = normalize_vector(features[0].cpu().tolist())
    return {"modelName": model_name, "vector": vector}


def embed_image(input_path: str) -> dict:
    ensure_existing_file(input_path)
    model, processor, model_name, torch = get_clip_bundle()

    try:
        from PIL import Image
    except ImportError as exc:
        raise RuntimeError("缺少 Pillow 依赖，请安装 backend/tools/search_ai.requirements.txt") from exc

    with Image.open(input_path) as image:
        image = image.convert("RGB")
        with torch.no_grad():
            inputs = processor(images=image, return_tensors="pt")
            features = resolve_feature_rows(model.get_image_features(**inputs))

    vector = normalize_vector(features[0].cpu().tolist())
    return {"modelName": model_name, "vector": vector}


def embed_video(input_path: str, ffmpeg_path: str) -> dict:
    ensure_existing_file(input_path)
    ffmpeg = prepare_ffmpeg_path(ffmpeg_path)

    with tempfile.TemporaryDirectory(prefix="mam-search-video-") as temp_dir:
        output_pattern = os.path.join(temp_dir, "frame-%03d.jpg")
        command = [
            ffmpeg,
            "-y",
            "-i",
            input_path,
            "-vf",
            "fps=1,scale=224:224:force_original_aspect_ratio=decrease",
            "-frames:v",
            "6",
            output_pattern,
        ]
        completed = subprocess.run(
            command,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
        )
        if completed.returncode != 0:
            stderr = (completed.stderr or "").strip()
            raise RuntimeError(f"视频关键帧提取失败: {stderr or completed.returncode}")

        frame_paths = sorted(str(path) for path in Path(temp_dir).glob("frame-*.jpg"))
        if not frame_paths:
            raise RuntimeError("未提取到视频关键帧")

        vectors = [embed_image(path)["vector"] for path in frame_paths]
        return {
            "modelName": embed_image(frame_paths[0])["modelName"],
            "vector": normalize_vector(average_vectors(vectors)),
        }


def describe_vector(vector_input, label_entries, top_k) -> dict:
    model, processor, model_name, torch = get_clip_bundle()

    vector = normalize_vector([float(value) for value in (vector_input or [])])
    if not vector:
        raise ValueError("vector is required")

    labels = []
    prompts = []
    for entry in label_entries or []:
        if not isinstance(entry, dict):
            continue
        label = str(entry.get("label", "")).strip()
        prompt = str(entry.get("prompt", "")).strip()
        if not label or not prompt:
            continue
        labels.append(label)
        prompts.append(prompt)

    if not prompts:
        raise ValueError("labels are required")

    with torch.no_grad():
        inputs = processor(text=prompts, return_tensors="pt", padding=True, truncation=True)
        features = resolve_feature_rows(model.get_text_features(**inputs))

    scored_labels = []
    for index, label in enumerate(labels):
        candidate = normalize_vector(features[index].cpu().tolist())
        scored_labels.append(
            {
                "label": label,
                "score": round(dot_product(vector, candidate), 6),
            }
        )

    scored_labels.sort(key=lambda item: float(item.get("score", 0.0)), reverse=True)

    resolved_top_k = int(top_k or 0)
    if resolved_top_k <= 0:
        resolved_top_k = min(6, len(scored_labels))

    return {
        "modelName": model_name,
        "labels": scored_labels[:resolved_top_k],
    }


def get_clip_bundle():
    model_name = os.getenv("MAM_SEARCH_CLIP_MODEL", "openai/clip-vit-base-patch32")
    cached = _CLIP_CACHE.get(model_name)
    if cached is not None:
        return cached

    try:
        import torch
        from transformers import AutoProcessor, CLIPModel
    except ImportError as exc:
        raise RuntimeError(
            "缺少 transformers/torch 依赖，请安装 backend/tools/search_ai.requirements.txt"
        ) from exc

    model = CLIPModel.from_pretrained(model_name)
    processor = AutoProcessor.from_pretrained(model_name)
    model.eval()

    cached = (model, processor, model_name, torch)
    _CLIP_CACHE[model_name] = cached
    return cached


def prepare_ffmpeg_path(ffmpeg_path: str) -> str:
    candidate = ffmpeg_path.strip() or os.getenv("MAM_SEARCH_FFMPEG", "ffmpeg")
    ffmpeg_dir = str(Path(candidate).expanduser().resolve().parent) if Path(candidate).suffix else ""
    if ffmpeg_dir and os.path.isdir(ffmpeg_dir):
        current_path = os.environ.get("PATH", "")
        if ffmpeg_dir not in current_path.split(os.pathsep):
            os.environ["PATH"] = ffmpeg_dir + os.pathsep + current_path
    return candidate


def normalize_vector(values):
    if not values:
        return []
    norm = math.sqrt(sum(float(value) * float(value) for value in values))
    if norm == 0:
        return [0.0 for _ in values]
    return [float(value) / norm for value in values]


def resolve_feature_rows(features):
    if hasattr(features, "text_embeds") and features.text_embeds is not None:
        features = features.text_embeds
    elif hasattr(features, "image_embeds") and features.image_embeds is not None:
        features = features.image_embeds
    elif hasattr(features, "pooler_output") and features.pooler_output is not None:
        features = features.pooler_output
    elif hasattr(features, "last_hidden_state") and features.last_hidden_state is not None:
        features = features.last_hidden_state
    elif isinstance(features, (list, tuple)) and features:
        features = resolve_feature_rows(features[0])

    if hasattr(features, "dim") and features.dim() > 2:
        features = features[:, 0]
    return features


def average_vectors(vectors):
    if not vectors:
        return []
    length = min(len(vector) for vector in vectors if vector)
    if length <= 0:
        return []
    averaged = []
    for index in range(length):
        averaged.append(sum(float(vector[index]) for vector in vectors) / len(vectors))
    return averaged


def dot_product(left, right):
    if not left or not right:
        return 0.0
    length = min(len(left), len(right))
    return sum(float(left[index]) * float(right[index]) for index in range(length))


def ensure_existing_file(input_path: str) -> None:
    if not input_path:
        raise ValueError("inputPath is required")
    if not os.path.isfile(input_path):
        raise FileNotFoundError(input_path)


if __name__ == "__main__":
    raise SystemExit(main())
