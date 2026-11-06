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
