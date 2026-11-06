"""Bancada evaluation service.

Started automatically by the Go backend (backend/evaluation.go). For each
(answer, evaluator) pair Go calls POST /evaluate and stores the score.

Run by hand for development:
    .venv/bin/python -m uvicorn main:app --port 8001 --reload
"""

from typing import Optional

from fastapi import FastAPI
from pydantic import BaseModel

import metrics
from i18n import language, tr

app = FastAPI(title="Bancada evaluator")


class EvaluatorConfig(BaseModel):
    kind: str  # ragas | llm_judge | embedding | rule
    metric: str = ""
    rubric: str = ""
    pattern: str = ""


class Item(BaseModel):
    question: str  # the prompt the model received
    answer: str  # what the model answered
    reference: str = ""  # expected answer from the dataset
    context: str = ""  # retrieved context (or the prompt itself)


class Endpoint(BaseModel):
    kind: str
    model: str
    base_url: str = ""
    api_key: str = ""


class EvaluateRequest(BaseModel):
    language: str = "pt-BR"  # "pt-BR" or "en": language of reasons and errors
    evaluator: EvaluatorConfig
    item: Item
    judge: Optional[Endpoint] = None
    embedding: Optional[Endpoint] = None


class EvaluateResponse(BaseModel):
    score: Optional[float] = None
    reason: str = ""
    error: str = ""


@app.get("/health")
def health():
    return {"ok": True}


@app.post("/evaluate", response_model=EvaluateResponse)
def evaluate(req: EvaluateRequest) -> EvaluateResponse:
    # Errors are returned as data (not HTTP errors) so one bad answer
    # never stops the evaluation of the others.
    language.set(req.language)
    try:
        score, reason = run_metric(req)
        return EvaluateResponse(score=round(score, 4), reason=reason)
    except Exception as exc:  # noqa: BLE001 - we want every failure reported to the UI
        return EvaluateResponse(error=str(exc))


def run_metric(req: EvaluateRequest) -> tuple[float, str]:
    ev = req.evaluator
    item = req.item.model_dump()
    judge = req.judge.model_dump() if req.judge else None
    embedder = req.embedding.model_dump() if req.embedding else None

    if ev.kind == "rule":
        return metrics.rule(ev.metric, item, ev.pattern)
    if ev.kind == "embedding":
        require(embedder, tr("modelo de embedding", "embedding model"))
        return metrics.embedding_similarity(embedder, item)
    if ev.kind == "llm_judge":
        require(judge, tr("modelo juiz", "judge model"))
        return metrics.llm_judge(judge, item, ev.rubric)
    if ev.kind == "ragas":
        require(judge, tr("modelo juiz", "judge model"))
        if ev.metric == "faithfulness":
            return metrics.faithfulness(judge, item)
        if ev.metric == "answer_relevancy":
            require(embedder, tr("modelo de embedding", "embedding model"))
            return metrics.answer_relevancy(judge, embedder, item)
        if ev.metric == "context_precision":
            return metrics.context_precision(judge, item)
    raise ValueError(tr(f"avaliador desconhecido: {ev.kind}/{ev.metric}", f"unknown evaluator: {ev.kind}/{ev.metric}"))


def require(value, label: str) -> None:
    if value is None:
        raise ValueError(tr(f"falta configurar o {label}", f"the {label} is not configured"))
