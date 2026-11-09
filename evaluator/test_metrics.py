from metrics import cosine, rule, split_context
from llm import parse_json


def item(answer, reference=""):
    return {"question": "q", "answer": answer, "reference": reference, "context": ""}


def test_exact_ignores_case_and_spaces():
    assert rule("exact", item("  Olá   Mundo ", "olá mundo"), "")[0] == 1.0
    assert rule("exact", item("outra coisa", "olá mundo"), "")[0] == 0.0


def test_contains_uses_pattern_or_reference():
    assert rule("contains", item("O prazo é de 24 meses"), "24 MESES")[0] == 1.0
    assert rule("contains", item("O prazo é de 24 meses", "prazo"), "")[0] == 1.0


def test_regex():
    assert rule("regex", item("Prazo: 36 meses"), r"(?i)prazo.*\d+\s*meses")[0] == 1.0
    assert rule("regex", item("sem prazo"), r"\d+ meses")[0] == 0.0


def test_cosine():
    assert abs(cosine([1, 0], [1, 0]) - 1) < 1e-9
    assert abs(cosine([1, 0], [0, 1])) < 1e-9


def test_parse_json_with_fences():
    assert parse_json('Claro!\n```json\n{"nota": 0.5}\n```') == {"nota": 0.5}


def test_split_context():
    assert split_context("a\n\nb\n---\nc") == ["a", "b", "c"]


def test_english_reasons():
    from i18n import language

    token = language.set("en")
    try:
        assert rule("contains", item("O prazo é de 24 meses"), "24 meses")[1] == "Contains “24 meses”."
    finally:
        language.reset(token)
    assert rule("contains", item("O prazo é de 24 meses"), "24 meses")[1] == "Contém “24 meses”."


# ---------- LLM judge with images ----------

import llm
import metrics

IMG = {"media_type": "image/png", "data": "aGVsbG8="}


class FakeResponse:
    status_code = 200

    def __init__(self, data):
        self._data = data
        self.text = ""

    def json(self):
        return self._data


def capture_chat(monkeypatch, kind, reply):
    """Runs the judge against a fake provider and returns the request body it sent."""
    sent = {}

    def fake_post(url, json=None, **kwargs):
        sent.update(json)
        return FakeResponse(reply)

    monkeypatch.setattr(llm.httpx, "post", fake_post)
    score, _ = metrics.llm_judge({"kind": kind, "model": "m"}, item("a cat", "cat"), "rubric", [IMG])
    assert score == 1.0
    return sent


ANSWER = '{"nota": 1, "motivo": "ok"}'


def test_judge_images_openai(monkeypatch):
    body = capture_chat(monkeypatch, "openai", {"choices": [{"message": {"content": ANSWER}}]})
    parts = body["messages"][1]["content"]
    assert parts[0]["type"] == "text" and "1 attached image" in parts[0]["text"]
    assert parts[1]["image_url"]["url"] == "data:image/png;base64,aGVsbG8="


def test_judge_images_anthropic(monkeypatch):
    body = capture_chat(monkeypatch, "anthropic", {"content": [{"type": "text", "text": ANSWER}]})
    parts = body["messages"][0]["content"]
    assert parts[0] == {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "aGVsbG8="}}
    assert parts[1]["type"] == "text"


def test_judge_images_gemini(monkeypatch):
    body = capture_chat(monkeypatch, "gemini", {"candidates": [{"content": {"parts": [{"text": ANSWER}]}}]})
    parts = body["contents"][0]["parts"]
    assert parts[0] == {"inlineData": {"mimeType": "image/png", "data": "aGVsbG8="}}
    assert "text" in parts[1]


def test_judge_images_ollama(monkeypatch):
    body = capture_chat(monkeypatch, "ollama", {"message": {"content": ANSWER}})
    assert body["messages"][1]["images"] == ["aGVsbG8="]


def test_judge_without_images_sends_plain_text(monkeypatch):
    sent = {}

    def fake_post(url, json=None, **kwargs):
        sent.update(json)
        return FakeResponse({"message": {"content": ANSWER}})

    monkeypatch.setattr(llm.httpx, "post", fake_post)
    metrics.llm_judge({"kind": "ollama", "model": "m"}, item("a cat"), "rubric")
    assert "images" not in sent["messages"][1]
    assert "attached image" not in sent["messages"][1]["content"]
