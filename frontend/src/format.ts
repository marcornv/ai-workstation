// format.ts: number/date formatting and labels shown in the UI, in the
// current language (see i18n.ts).

import type { EvaluatorKind, ProviderKind, Run } from "./api";
import { locale, t } from "./i18n";

export const fmtInt = (n: number) => n.toLocaleString(locale());

export const fmtScore = (n: number | null | undefined) =>
  n == null ? "—" : n.toLocaleString(locale(), { minimumFractionDigits: 2, maximumFractionDigits: 2 });

export const fmtUSD = (n: number | null | undefined) =>
  n == null
    ? "—"
    : t("US$ ", "$") + n.toLocaleString(locale(), { minimumFractionDigits: 2, maximumFractionDigits: n < 0.1 && n > 0 ? 4 : 2 });

export const fmtSeconds = (ms: number) =>
  (ms / 1000).toLocaleString(locale(), { minimumFractionDigits: 1, maximumFractionDigits: 1 }) + " s";

// 1234567 -> "1,2 mi" / "1.2M", 45000 -> "45 mil" / "45K"
export function fmtCompact(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toLocaleString(locale(), { maximumFractionDigits: 1 }) + t(" mi", "M");
  if (n >= 10_000) return Math.round(n / 1000).toLocaleString(locale()) + t(" mil", "K");
  return fmtInt(n);
}

export function fmtAgo(iso: string | null | undefined): string {
  if (!iso) return "—";
  const seconds = (Date.now() - new Date(iso).getTime()) / 1000;
  if (seconds < 60) return t("agora", "just now");
  if (seconds < 3600) return t(`há ${Math.floor(seconds / 60)} min`, `${Math.floor(seconds / 60)} min ago`);
  if (seconds < 86400) return t(`há ${Math.floor(seconds / 3600)} h`, `${Math.floor(seconds / 3600)} h ago`);
  const days = Math.floor(seconds / 86400);
  if (days === 1) return t("ontem", "yesterday");
  return t(`há ${days} dias`, `${days} days ago`);
}

export const providerKindLabel: Record<ProviderKind, string> = {
  openai: "OpenAI",
  anthropic: "Anthropic",
  gemini: "Google Gemini",
  ollama: "Ollama (local)",
  openai_compatible: t("Compatível com OpenAI", "OpenAI-compatible"),
  typesafe: t("TypeSafe (Jev, decisão)", "TypeSafe (Jev, decision)"),
};

export const providerTag: Record<ProviderKind, string> = {
  openai: "OA",
  anthropic: "AN",
  gemini: "GG",
  ollama: "OL",
  openai_compatible: "API",
  typesafe: "TS",
};

// Example model names shown as placeholders when adding a model.
export const modelPlaceholder: Record<ProviderKind, string> = {
  openai: "gpt-4o-mini",
  anthropic: "claude-sonnet-5-5",
  gemini: "gemini-2.5-flash",
  ollama: "llama3.1:8b",
  openai_compatible: t("nome-do-modelo", "model-name"),
  typesafe: "jev-latest",
};

export const evaluatorKindLabel: Record<EvaluatorKind, string> = {
  ragas: "Ragas",
  llm_judge: t("Juiz LLM", "LLM judge"),
  embedding: "Embedding",
  rule: t("Regra", "Rule"),
  decision: t("Motor de decisão", "Decision engine"),
};

export const metricLabel: Record<string, string> = {
  faithfulness: t("Fidelidade (faithfulness)", "Faithfulness"),
  answer_relevancy: t("Relevância da resposta (answer relevancy)", "Answer relevancy"),
  context_precision: t("Precisão do contexto (context precision)", "Context precision"),
  exact: t("Igual à referência", "Same as the reference"),
  contains: t("Contém texto", "Contains text"),
  regex: t("Expressão regular", "Regular expression"),
};

export const useCaseLabel: Record<string, string> = {
  free: t("Prompt livre", "Free prompt"),
  rag: t("Pergunta e resposta com contexto (RAG)", "Question answering with context (RAG)"),
  classification: t("Classificação", "Classification"),
  extraction: t("Extração estruturada (JSON)", "Structured extraction (JSON)"),
};

export function runStatus(run: Run | null): { label: string; cls: string } {
  if (!run) return { label: t("Rascunho", "Draft"), cls: "draft" };
  switch (run.status) {
    case "running":
      return { label: t("Rodando", "Running"), cls: "run" };
    case "done":
      return run.errors > 0
        ? { label: t(`Concluído · ${run.errors} erro(s)`, `Done · ${run.errors} error(s)`), cls: "warn" }
        : { label: t("Concluído", "Done"), cls: "ok" };
    case "canceled":
      return { label: t("Cancelado", "Canceled"), cls: "warn" };
    default:
      return { label: t("Falhou", "Failed"), cls: "err" };
  }
}
