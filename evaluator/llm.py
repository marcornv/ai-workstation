"""Minimal clients for calling judge and embedding models.

Go sends us an "endpoint" for each model: {kind, model, base_url, api_key}.
kind is one of: openai, openai_compatible, anthropic, gemini, ollama.
Plain HTTP (httpx) keeps this file small and easy to read, mirroring
backend/providers.go.
"""

import json
import re

import httpx

from i18n import tr

TIMEOUT = httpx.Timeout(180.0)


def _base(endpoint: dict, default: str) -> str:
    return (endpoint.get("base_url") or default).rstrip("/")


def _raise_for_status(resp: httpx.Response) -> None:
    if resp.status_code >= 300:
        raise RuntimeError(tr(f"HTTP {resp.status_code} do provedor: ", f"HTTP {resp.status_code} from the provider: ") + resp.text[:300])


def chat(endpoint: dict, system: str, user: str, max_tokens: int = 2048, images: list[dict] | None = None) -> str:
    """Send one prompt and return the text answer.

    images: optional [{"media_type": "image/png", "data": <base64>}], attached
    to the user message in each provider's format (as in backend/providers.go).
    The model must have vision.
    """
    kind = endpoint["kind"]
    model = endpoint["model"]
    key = endpoint.get("api_key") or ""
    images = images or []

    if kind in ("openai", "openai_compatible"):
        body = {
            "model": model,
            "messages": [{"role": "system", "content": system}, {"role": "user", "content": _openai_content(user, images)}],
        }
        body["max_completion_tokens" if kind == "openai" else "max_tokens"] = max_tokens
        headers = {"Authorization": f"Bearer {key}"} if key else {}
        resp = httpx.post(f"{_base(endpoint, 'https://api.openai.com')}/v1/chat/completions",
                          json=body, headers=headers, timeout=TIMEOUT)
        _raise_for_status(resp)
        return resp.json()["choices"][0]["message"]["content"] or ""

    if kind == "anthropic":
        body = {
            "model": model,
            "max_tokens": max_tokens,
            "system": system,
            "messages": [{"role": "user", "content": _anthropic_content(user, images)}],
        }
        headers = {"x-api-key": key, "anthropic-version": "2026-10-21"}
        resp = httpx.post(f"{_base(endpoint, 'https://api.anthropic.com')}/v1/messages",
                          json=body, headers=headers, timeout=TIMEOUT)
        _raise_for_status(resp)
        data = resp.json()
        if data.get("stop_reason") == "refusal":
            raise RuntimeError(tr("o modelo juiz recusou responder", "the judge model refused to answer"))
        return "".join(b.get("text", "") for b in data["content"] if b.get("type") == "text")

    if kind == "gemini":
        body = {
            "systemInstruction": {"parts": [{"text": system}]},
            "contents": [{"role": "user", "parts": _gemini_parts(user, images)}],
            "generationConfig": {"temperature": 0, "maxOutputTokens": max_tokens},
        }
        url = f"{_base(endpoint, 'https://generativelanguage.googleapis.com')}/v1beta/models/{model}:generateContent"
        resp = httpx.post(url, json=body, headers={"x-goog-api-key": key}, timeout=TIMEOUT)
        _raise_for_status(resp)
        candidates = resp.json().get("candidates") or []
        if not candidates:
            raise RuntimeError(tr("o Gemini não retornou resposta", "Gemini returned no answer"))
        return "".join(p.get("text", "") for p in candidates[0]["content"].get("parts", []))

    if kind == "ollama":
        body = {
            "model": model,
            "stream": False,
            "options": {"temperature": 0, "num_predict": max_tokens},
            "messages": [{"role": "system", "content": system}, _ollama_user(user, images)],
        }
        resp = httpx.post(f"{_base(endpoint, 'http://localhost:11434')}/api/chat", json=body, timeout=TIMEOUT)
        _raise_for_status(resp)
        return resp.json()["message"]["content"]

    raise ValueError(tr(f"tipo de provedor desconhecido: {kind}", f"unknown provider kind: {kind}"))


def _openai_content(user: str, images: list[dict]):
    if not images:
        return user
    parts = [{"type": "text", "text": user}]
    for img in images:
        parts.append({"type": "image_url", "image_url": {"url": f"data:{img['media_type']};base64,{img['data']}"}})
    return parts


def _anthropic_content(user: str, images: list[dict]):
    if not images:
        return user
    # Images first, then the text: Claude works best with that order.
    parts = [{"type": "image", "source": {"type": "base64", "media_type": img["media_type"], "data": img["data"]}}
             for img in images]
    return parts + [{"type": "text", "text": user}]


def _gemini_parts(user: str, images: list[dict]) -> list[dict]:
    parts = [{"inlineData": {"mimeType": img["media_type"], "data": img["data"]}} for img in images]
    return parts + [{"text": user}]


def _ollama_user(user: str, images: list[dict]) -> dict:
    message = {"role": "user", "content": user}
    if images:
        message["images"] = [img["data"] for img in images]
    return message


def embed(endpoint: dict, texts: list[str]) -> list[list[float]]:
    """Return one embedding vector per text."""
    kind = endpoint["kind"]
    model = endpoint["model"]
    key = endpoint.get("api_key") or ""

    if kind in ("openai", "openai_compatible"):
        headers = {"Authorization": f"Bearer {key}"} if key else {}
        resp = httpx.post(f"{_base(endpoint, 'https://api.openai.com')}/v1/embeddings",
                          json={"model": model, "input": texts}, headers=headers, timeout=TIMEOUT)
        _raise_for_status(resp)
        return [item["embedding"] for item in resp.json()["data"]]

    if kind == "ollama":
        resp = httpx.post(f"{_base(endpoint, 'http://localhost:11434')}/api/embed",
                          json={"model": model, "input": texts}, timeout=TIMEOUT)
        _raise_for_status(resp)
        return resp.json()["embeddings"]

    if kind == "gemini":
        url = f"{_base(endpoint, 'https://generativelanguage.googleapis.com')}/v1beta/models/{model}:batchEmbedContents"
        body = {"requests": [{"model": f"models/{model}", "content": {"parts": [{"text": t}]}} for t in texts]}
        resp = httpx.post(url, json=body, headers={"x-goog-api-key": key}, timeout=TIMEOUT)
        _raise_for_status(resp)
        return [e["values"] for e in resp.json()["embeddings"]]

    raise ValueError(tr(f"o provedor '{kind}' não oferece embeddings aqui; use OpenAI, Gemini ou Ollama",
                        f"provider '{kind}' has no embeddings here; use OpenAI, Gemini or Ollama"))


def parse_json(text: str):
    """Pull the first JSON object or array out of a model answer.

    Models often wrap JSON in ```json fences or add a sentence around it.
    """
    match = re.search(r"(\{.*\}|\[.*\])", text, re.DOTALL)
    if not match:
        raise ValueError(tr("o juiz não respondeu em JSON: ", "the judge did not answer in JSON: ") + text[:200])
    return json.loads(match.group(1))
