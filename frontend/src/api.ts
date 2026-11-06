// api.ts: the only file that talks to the Go backend.
// Types mirror the JSON written by backend/*.go.

import { t } from "./i18n";

// ---------- Types ----------

export type ProviderKind = "openai" | "anthropic" | "gemini" | "ollama" | "openai_compatible" | "typesafe";

export interface ModelConfig {
  model: string;
  provider: string;
  enabled: boolean;
  temperature: number | null;
  max_tokens: number;
}

export interface RunSettings {
  repetitions: number;
  concurrency: number;
  timeout_s: number;
}

export interface Experiment {
  id: string;
  name: string;
  description: string;
  use_case: string;
  models: ModelConfig[];
  prompt: { system: string; user: string };
  dataset: { file: string; reference_column: string; context_column: string; image_columns: string[] };
  run: RunSettings;
  evaluators: string[];
}

export interface Run {
  id: number;
  experiment: string;
  status: "running" | "done" | "canceled" | "failed";
  total: number;
  done: number;
  errors: number;
  started_at: string;
  finished_at: string | null;
}

export interface ExperimentListItem extends Experiment {
  inputs: number;
  enabled_models: number;
  last_run: Run | null;
  mean_score: number | null;
  changed: boolean;
}

export interface Estimate {
  inputs: number;
  calls: number;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
  paid_calls: number;
  models_without_price: string[];
}

export interface Result {
  id: number;
  run_id: number;
  input_index: number;
  repetition: number;
  provider: string;
  model: string;
  user_prompt: string;
  reference: string;
  context: string;
  images: string[];
  output: string;
  input_tokens: number;
  output_tokens: number;
  latency_ms: number;
  cost_usd: number | null;
  error: string;
}

export interface Score {
  result_id: number;
  evaluator_id: string;
  score: number | null;
  reason: string;
  error: string;
}

export interface ModelSummary {
  model: string;
  provider: string;
  calls: number;
  errors: number;
  input_tokens: number;
  output_tokens: number;
  latency_p50_ms: number;
  latency_p95_ms: number;
  cost_usd: number | null;
  mean_score: number | null;
}

export interface RunSummary {
  calls: number;
  errors: number;
  input_tokens: number;
  output_tokens: number;
  latency_p50_ms: number;
  latency_p95_ms: number;
  cost_usd: number;
  cost_complete: boolean;
  models: ModelSummary[];
}

export interface RunResults {
  run: Run;
  summary: RunSummary;
  results: Result[];
  scores: Score[];
}

export type EvaluatorKind = "ragas" | "llm_judge" | "embedding" | "rule" | "decision";

// A question for a decision model (see backend/decisions.go).
export interface DecisionOption {
  label: string;
  description: string;
  pass: boolean; // choice: this label counts as a good answer
}

export interface DecisionQuestion {
  type: "noul" | "choice" | "score";
  instructions: string;
  true_means: string;
  false_means: string;
  options: DecisionOption[] | null; // choice labels, or score levels (lowest first)
}

export interface DecisionAnswer {
  type: string;
  noul?: number;
  choice?: string;
  score?: number;
  probabilities?: Record<string, number>;
  confidence?: number;
}

export interface DecisionTestResult {
  score: number;
  reason: string;
  passed: boolean;
  answer: DecisionAnswer;
  state: string;
}

export interface Evaluator {
  id: string;
  name: string;
  description: string;
  kind: EvaluatorKind;
  metric: string;
  threshold: number;
  judge_provider: string;
  judge_model: string;
  embedding_provider: string;
  embedding_model: string;
  rubric: string;
  pattern: string;
  // Only for kind "decision".
  decision_provider?: string;
  decision_model?: string;
  question?: DecisionQuestion;
  state_template?: string;
  send_images?: boolean;
  problems?: string[];
}

// ---------- Evaluator spreadsheet tests ----------

export interface SheetColumns {
  answer: string;
  question: string;
  reference: string;
  context: string;
  images: string;
  human: string; // human verdict column (sim/não, 1/0…)
}

export interface EvaluatorTest {
  id: number;
  evaluator_id: string;
  dataset: string;
  status: "running" | "done" | "canceled";
  total: number;
  done: number;
  started_at: string;
  finished_at: string | null;
}

export interface TestRow {
  row_index: number;
  question: string;
  answer: string;
  reference: string;
  human: string;
  score: number | null;
  reason: string;
  error: string;
  passed: boolean | null; // score >= threshold
  agrees: boolean | null; // evaluator and your verdict agree
}

export interface TestSummary {
  rows: number;
  scored: number;
  errors: number;
  mean_score: number | null;
  pass_rate: number | null;
  threshold: number;
  labeled: number;
  agreement: number | null;
  both_pass: number;
  only_eval: number;
  only_human: number;
  both_fail: number;
  unlabeled: number;
  best_threshold: number | null;
  best_agreement: number | null;
}

export interface EvaluatorTestDetail {
  test: EvaluatorTest;
  columns: SheetColumns;
  evaluator: Evaluator;
  summary: TestSummary;
  rows: TestRow[];
}

export interface EvalProgress {
  running: boolean;
  total: number;
  done: number;
  error: string;
}

export interface RunEvaluation {
  progress: EvalProgress;
  models: string[];
  matrix: Record<string, Record<string, { n: number; errors: number; mean: number | null }>>;
  answers: number;
}

export interface ProviderView {
  name: string;
  kind: ProviderKind;
  api_key_env: string;
  base_url: string;
  key_set: boolean;
  key_masked: string;
}

export interface Settings {
  config: {
    language: "pt-BR" | "en";
    server: { port: number; evaluator_url: string; open_browser: boolean };
    defaults: { run: RunSettings; judge_provider: string; judge_model: string };
    paths: Record<string, string>;
    pricing: Record<string, { input: number; output: number }>;
  };
  providers: ProviderView[];
  file: string;
  file_path: string;
}

export interface Health {
  api: boolean;
  api_port: number;
  evaluator: boolean;
  evaluator_url: string;
  storage: boolean;
  storage_path: string;
}

export interface DatasetPreview {
  file: string;
  columns: string[];
  rows: Record<string, string>[];
  count: number;
  image_columns_guess: string[];
}

// imageURL turns an image reference from a dataset (path inside data/ or URL)
// into something an <img> tag can show.
export function imageURL(ref: string): string {
  if (/^https?:\/\//.test(ref)) return ref;
  return "/api/files/" + ref.split("/").map(encodeURIComponent).join("/");
}

// ---------- Request helper ----------

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, headers: {} };
  if (body instanceof FormData) {
    init.body = body;
  } else if (body !== undefined) {
    init.body = JSON.stringify(body);
    init.headers = { "Content-Type": "application/json" };
  }
  let resp: Response;
  try {
    resp = await fetch(path, init);
  } catch {
    throw new Error(t("Não foi possível falar com o Bancada. O programa ainda está aberto?", "Could not reach Bancada. Is the program still running?"));
  }
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) {
    throw new Error(data.error || t(`Erro ${resp.status}`, `Error ${resp.status}`));
  }
  return data as T;
}

const enc = encodeURIComponent;

// ---------- Endpoints ----------

export const api = {
  health: () => request<Health>("GET", "/api/health"),

  settings: () => request<Settings>("GET", "/api/settings"),
  setLanguage: (language: "pt-BR" | "en") => request<Settings>("PUT", "/api/settings/language", { language }),
  saveSettingsFile: (yaml: string) => request<Settings>("PUT", "/api/settings/file", { yaml }),
  saveProvider: (name: string, p: { kind: ProviderKind; api_key_env: string; base_url: string }) =>
    request<Settings>("PUT", `/api/providers/${enc(name)}`, p),
  deleteProvider: (name: string) => request<Settings>("DELETE", `/api/providers/${enc(name)}`),
  saveKey: (name: string, key: string) => request<Settings>("PUT", `/api/providers/${enc(name)}/key`, { key }),
  testProvider: (name: string) =>
    request<{ ok: boolean; ms?: number; models?: number; error?: string }>("POST", `/api/providers/${enc(name)}/test`),
  pullModel: (name: string, model: string) => request<{ ok: boolean }>("POST", `/api/providers/${enc(name)}/pull`, { model }),
  providerModels: (name: string) => request<{ models: string[] }>("GET", `/api/providers/${enc(name)}/models`),

  experiments: () => request<ExperimentListItem[]>("GET", "/api/experiments"),
  createExperiment: (body: { name: string; description: string; use_case: string; duplicate_from?: string }) =>
    request<Experiment>("POST", "/api/experiments", body),
  experiment: (id: string) =>
    request<{ experiment: Experiment; file_path: string; changed: boolean }>("GET", `/api/experiments/${enc(id)}`),
  saveExperiment: (exp: Experiment) =>
    request<{ experiment: Experiment; file_path: string; changed: boolean }>("PUT", `/api/experiments/${enc(exp.id)}`, exp),
  deleteExperiment: (id: string) => request<{ ok: boolean }>("DELETE", `/api/experiments/${enc(id)}`),
  experimentFile: (id: string) => request<{ yaml: string; path: string }>("GET", `/api/experiments/${enc(id)}/file`),
  saveExperimentFile: (id: string, yaml: string) => request("PUT", `/api/experiments/${enc(id)}/file`, { yaml }),
  estimate: (id: string, limit = 0) =>
    request<{ ready: boolean; problem?: string; estimate?: Estimate }>("POST", `/api/experiments/${enc(id)}/estimate`, { limit }),
  runs: (id: string) => request<Run[]>("GET", `/api/experiments/${enc(id)}/runs`),
  startRun: (id: string, limit = 0) => request<{ run_id: number }>("POST", `/api/experiments/${enc(id)}/runs`, { limit }),

  run: (runId: number) => request<Run>("GET", `/api/runs/${runId}`),
  cancelRun: (runId: number) => request("POST", `/api/runs/${runId}/cancel`),
  runResults: (runId: number) => request<RunResults>("GET", `/api/runs/${runId}/results`),
  exportURL: (runId: number) => `/api/runs/${runId}/export.csv`,
  evaluateRun: (runId: number, evaluators: string[]) =>
    request<EvalProgress>("POST", `/api/runs/${runId}/evaluate`, { evaluators }),
  runEvaluation: (runId: number) => request<RunEvaluation>("GET", `/api/runs/${runId}/evaluation`),

  datasets: () => request<string[]>("GET", "/api/datasets"),
  uploadDataset: (file: File) => {
    const form = new FormData();
    form.append("file", file);
    return request<{ file: string }>("POST", "/api/datasets", form);
  },
  uploadImages: (name: string, files: FileList) => {
    const form = new FormData();
    form.append("name", name);
    for (const f of Array.from(files)) form.append("files", f);
    return request<{ file: string; image_columns: string[]; count: number }>("POST", "/api/datasets/images", form);
  },
  pasteDataset: (name: string, text: string) => request<{ file: string }>("POST", "/api/datasets", { name, text }),
  previewDataset: (file: string, limit = 5) =>
    request<DatasetPreview>("GET", `/api/datasets/${enc(file)}?limit=${limit}`),

  evaluators: (experimentId?: string) =>
    request<Evaluator[]>("GET", "/api/evaluators" + (experimentId ? `?experiment=${enc(experimentId)}` : "")),
  createEvaluator: (ev: Omit<Evaluator, "id">) => request<Evaluator>("POST", "/api/evaluators", ev),
  saveEvaluator: (ev: Evaluator) => request<Evaluator>("PUT", `/api/evaluators/${enc(ev.id)}`, ev),
  testEvaluator: (evaluator: Omit<Evaluator, "id">, item: { question: string; answer: string; reference: string }) =>
    request<DecisionTestResult>("POST", "/api/evaluators/test", { evaluator, item }),
  evaluatorTests: (id: string) => request<EvaluatorTest[]>("GET", `/api/evaluators/${enc(id)}/tests`),
  startEvaluatorTest: (id: string, file: string, columns: SheetColumns) =>
    request<{ test_id: number }>("POST", `/api/evaluators/${enc(id)}/tests`, { file, columns }),
  evaluatorTest: (testId: number) => request<EvaluatorTestDetail>("GET", `/api/evaluator-tests/${testId}`),
  cancelEvaluatorTest: (testId: number) => request("POST", `/api/evaluator-tests/${testId}/cancel`),
  deleteEvaluatorTest: (testId: number) => request("DELETE", `/api/evaluator-tests/${testId}`),
  evaluatorTestExportURL: (testId: number) => `/api/evaluator-tests/${testId}/export.csv`,
  deleteEvaluator: (id: string) => request("DELETE", `/api/evaluators/${enc(id)}`),
};
