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


def chat(endpoint: dict, system: str, user: str, max_tokens: int = 2048) -> str:
    """Send one prompt and return the text answer."""
    kind = endpoint["kind"]
    model = endpoint["model"]
    key = endpoint.get("api_key") or ""

    if kind in ("openai", "openai_compatible"):
        body = {
            "model": model,
            "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}],
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
            "messages": [{"role": "user", "content": user}],
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
            "contents": [{"role": "user", "parts": [{"text": user}]}],
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
            "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}],
        }
        resp = httpx.post(f"{_base(endpoint, 'http://localhost:11434')}/api/chat", json=body, timeout=TIMEOUT)
        _raise_for_status(resp)
        return resp.json()["message"]["content"]

    raise ValueError(tr(f"tipo de provedor desconhecido: {kind}", f"unknown provider kind: {kind}"))


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
