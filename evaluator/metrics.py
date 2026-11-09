"""Evaluation metrics. Every function returns (score between 0 and 1, reason).

The three "ragas" metrics follow the method of the Ragas library
(https://docs.ragas.io), implemented directly so the steps are easy to read
and work with any judge model:

  faithfulness       split the answer into statements, check each against the context
  answer_relevancy   generate questions from the answer, compare them to the real question
  context_precision  check whether the useful context chunks come first
"""

import math
import re

from i18n import tr
from llm import chat, embed, parse_json

# ---------- Ragas-style metrics ----------


def faithfulness(judge: dict, item: dict) -> tuple[float, str]:
    statements_answer = parse_json(chat(
        judge,
        "You break answers into short, self-contained factual statements. Reply with JSON only.",
        f"Question:\n{item['question']}\n\nAnswer:\n{item['answer']}\n\n"
        'Return {"statements": ["...", "..."]} with every factual claim made in the answer.',
    ))
    statements = statements_answer.get("statements", [])
    if not statements:
        return 1.0, tr("A resposta não faz afirmações verificáveis.", "The answer makes no verifiable claims.")

    numbered = "\n".join(f"{i + 1}. {s}" for i, s in enumerate(statements))
    verdicts = parse_json(chat(
        judge,
        "You check whether statements can be directly inferred from a context. Reply with JSON only.",
        f"Context:\n{item['context']}\n\nStatements:\n{numbered}\n\n"
        'Return {"verdicts": [{"statement": "...", "supported": 1 or 0, "reason": "..."}]}, one per statement, same order.',
    )).get("verdicts", [])

    supported = sum(1 for v in verdicts if int(v.get("supported", 0)) == 1)
    unsupported = [v.get("statement", "") for v in verdicts if int(v.get("supported", 0)) != 1]
    score = supported / len(statements)
    reason = tr(f"{supported} de {len(statements)} afirmações sustentadas pelo contexto.",
                f"{supported} of {len(statements)} claims supported by the context.")
    if unsupported:
        reason += tr(" Não sustentadas: ", " Not supported: ") + "; ".join(unsupported[:3])
    return score, reason


def answer_relevancy(judge: dict, embedder: dict, item: dict, n_questions: int = 3) -> tuple[float, str]:
    generated = parse_json(chat(
        judge,
        "You read an answer and guess which question it is answering. Reply with JSON only.",
        f"Answer:\n{item['answer']}\n\n"
        f'Return {{"questions": [{n_questions} different questions this answer responds to], '
        '"noncommittal": 1 if the answer is evasive or vague (e.g. "I don\'t know"), else 0}.',
    ))
    if int(generated.get("noncommittal", 0)) == 1:
        return 0.0, tr("A resposta é evasiva ou vaga.", "The answer is evasive or vague.")
    questions = generated.get("questions", [])
    if not questions:
        return 0.0, tr("Não foi possível gerar perguntas a partir da resposta.", "Could not generate questions from the answer.")

    vectors = embed(embedder, [item["question"], *questions])
    original, others = vectors[0], vectors[1:]
    similarities = [cosine(original, v) for v in others]
    score = max(0.0, sum(similarities) / len(similarities))
    return score, tr("Perguntas reconstruídas: ", "Reconstructed questions: ") + " | ".join(questions)


def context_precision(judge: dict, item: dict) -> tuple[float, str]:
    chunks = split_context(item["context"])
    useful = []
    for chunk in chunks:
        verdict = parse_json(chat(
            judge,
            "You decide whether a context chunk was useful to reach a given answer. Reply with JSON only.",
            f"Question:\n{item['question']}\n\nExpected answer:\n{item['reference']}\n\nContext chunk:\n{chunk}\n\n"
            'Return {"useful": 1 or 0, "reason": "..."}.',
        ))
        useful.append(int(verdict.get("useful", 0)) == 1)

    # Average precision: rewards useful chunks that appear early in the list.
    hits, total = 0, 0.0
    for k, is_useful in enumerate(useful, start=1):
        if is_useful:
            hits += 1
            total += hits / k
    if hits == 0:
        return 0.0, tr(f"Nenhum dos {len(chunks)} trechos de contexto foi útil.", f"None of the {len(chunks)} context chunks was useful.")
    return total / hits, tr(f"{hits} de {len(chunks)} trechos úteis.", f"{hits} of {len(chunks)} chunks useful.")


def split_context(context: str) -> list[str]:
    """Contexts are separated by blank lines or '---' lines."""
    parts = re.split(r"\n\s*(?:---+)?\s*\n", context)
    return [p.strip() for p in parts if p.strip()] or [context]


# ---------- LLM judge with a custom rubric ----------


def llm_judge(judge: dict, item: dict, rubric: str, images: list[dict] | None = None) -> tuple[float, str]:
    reference = f"\n\nExpected answer (reference):\n{item['reference']}" if item.get("reference") else ""
    # With images, the judge sees the same pictures the model saw.
    attached = f"\n\nThe input also included the {len(images)} attached image(s)." if images else ""
    result = parse_json(chat(
        judge,
        "You are a strict evaluator. Grade the answer using the rubric. "
        'Reply with JSON only: {"nota": number between 0 and 1, "motivo": "short reason in '
        + tr("Portuguese", "English") + '"}.',
        f"Rubric:\n{rubric}\n\nInput given to the model:\n{item['question']}{attached}{reference}\n\nModel answer:\n{item['answer']}",
        images=images,
    ))
    score = result.get("nota", result.get("score"))
    reason = result.get("motivo", result.get("reason", ""))
    if score is None:
        raise ValueError(tr("o juiz não retornou o campo 'nota'", "the judge did not return the 'nota' field"))
    score = float(score)
    if score > 1:  # some judges answer 0-10 or 0-100 despite the instructions
        score = score / 10 if score <= 10 else score / 100
    return min(max(score, 0.0), 1.0), str(reason)


# ---------- Embedding similarity ----------


def embedding_similarity(embedder: dict, item: dict) -> tuple[float, str]:
    answer_vec, reference_vec = embed(embedder, [item["answer"], item["reference"]])
    similarity = cosine(answer_vec, reference_vec)
    return max(0.0, similarity), tr("Similaridade de cosseno: ", "Cosine similarity: ") + f"{similarity:.3f}"


def cosine(a: list[float], b: list[float]) -> float:
    dot = sum(x * y for x, y in zip(a, b))
    norm = math.sqrt(sum(x * x for x in a)) * math.sqrt(sum(y * y for y in b))
    return dot / norm if norm else 0.0


# ---------- Rules (no model, no cost) ----------


def rule(metric: str, item: dict, pattern: str) -> tuple[float, str]:
    answer = item["answer"]
    if metric == "exact":
        ok = normalize(answer) == normalize(item["reference"])
        return (1.0, tr("Igual à referência.", "Same as the reference.")) if ok else (0.0, tr("Diferente da referência.", "Different from the reference."))
    if metric == "contains":
        needle = pattern or item["reference"]
        ok = normalize(needle) in normalize(answer)
        return (1.0, tr(f"Contém “{needle}”.", f"Contains “{needle}”.")) if ok else (0.0, tr(f"Não contém “{needle}”.", f"Does not contain “{needle}”."))
    if metric == "regex":
        ok = re.search(pattern, answer) is not None
        return (1.0, tr("O padrão foi encontrado.", "The pattern was found.")) if ok else (0.0, tr("O padrão não foi encontrado.", "The pattern was not found."))
    raise ValueError(tr(f"regra desconhecida: {metric}", f"unknown rule: {metric}"))


def normalize(text: str) -> str:
    return " ".join(text.casefold().split())
