// Form to create or edit an evaluator. Used on the Avaliadores page and in the
// experiment's Avaliação tab.

import { useEffect, useState } from "react";
import { api, type Evaluator, type EvaluatorKind } from "../api";
import { useLoad } from "../hooks";
import { evaluatorKindLabel, metricLabel } from "../format";
import { DecisionFields } from "./DecisionFields";
import { t } from "../i18n";

export const evaluatorTemplates: Record<string, Omit<Evaluator, "id">> = {
  ragas: {
    name: t("Fidelidade", "Faithfulness"),
    description: t(
      "Mede se cada afirmação da resposta é sustentada pelo texto de entrada. Detecta invenções.",
      "Checks whether each claim in the answer is supported by the input text. Catches made-up facts.",
    ),
    kind: "ragas",
    metric: "faithfulness",
    threshold: 0.7,
    judge_provider: "",
    judge_model: "",
    embedding_provider: "",
    embedding_model: "",
    rubric: "",
    pattern: "",
  },
  llm_judge: {
    name: t("Correção por rubrica", "Rubric correctness"),
    description: t("Um modelo dá uma nota seguindo critérios escritos por você.", "A model scores the answer following criteria you write."),
    kind: "llm_judge",
    metric: "",
    threshold: 0.7,
    judge_provider: "",
    judge_model: "",
    embedding_provider: "",
    embedding_model: "",
    rubric:
      t(
        "Dê uma nota de 0 a 1 para a resposta.\n1 = correta, completa e sem invenções.\n0 = errada ou fora do assunto.\nPenalize afirmações que não estejam no input.",
        "Score the answer from 0 to 1.\n1 = correct, complete and nothing made up.\n0 = wrong or off-topic.\nPenalize claims that are not in the input.",
      ),
    pattern: "",
  },
  embedding: {
    name: t("Similaridade com a referência", "Similarity to the reference"),
    description: t("Compara o significado da resposta com a resposta esperada do dataset.", "Compares the meaning of the answer with the expected answer in the dataset."),
    kind: "embedding",
    metric: "",
    threshold: 0.75,
    judge_provider: "",
    judge_model: "",
    embedding_provider: "ollama",
    embedding_model: "nomic-embed-text",
    rubric: "",
    pattern: "",
  },
  decision: {
    name: t("Resposta correta (decisão)", "Correct answer (decision)"),
    description: t(
      "Um modelo de decisão diz se a resposta está correta, com uma probabilidade. Rápido e sem gerar texto.",
      "A decision model says whether the answer is correct, with a probability. Fast, and generates no text.",
    ),
    kind: "decision",
    metric: "",
    threshold: 0.5,
    judge_provider: "",
    judge_model: "",
    embedding_provider: "",
    embedding_model: "",
    rubric: "",
    pattern: "",
    decision_provider: "ollama",
    decision_model: "laya",
    question: {
      type: "noul",
      instructions: "Is the model answer correct and consistent with the expected answer?",
      true_means: "The answer is correct and agrees with the expected answer",
      false_means: "The answer is wrong, incomplete or off-topic",
      options: [],
    },
    state_template: "",
    send_images: false,
  },
  rule: {
    name: t("Contém o termo esperado", "Contains the expected term"),
    description: t("Passa ou falha conforme a resposta contenha o padrão. Rápido e sem custo.", "Passes or fails depending on whether the answer contains the pattern. Fast and free."),
    kind: "rule",
    metric: "contains",
    threshold: 1,
    judge_provider: "",
    judge_model: "",
    embedding_provider: "",
    embedding_model: "",
    rubric: "",
    pattern: "",
  },
};

const metricHelp: Record<string, string> = {
  faithfulness: t(
    "Quebra a resposta em afirmações e verifica cada uma contra o contexto (ou o input, se não houver coluna de contexto).",
    "Splits the answer into claims and checks each one against the context (or the input, if there is no context column).",
  ),
  answer_relevancy: t(
    "Gera perguntas a partir da resposta e compara com o input original. Precisa de juiz e de modelo de embedding.",
    "Generates questions from the answer and compares them with the original input. Needs a judge and an embedding model.",
  ),
  context_precision: t(
    "Verifica se os trechos de contexto úteis aparecem primeiro. Precisa das colunas de contexto e de resposta esperada.",
    "Checks whether the useful context chunks come first. Needs the context and expected answer columns.",
  ),
  exact: t("Nota 1 se a resposta for igual à resposta esperada (ignora maiúsculas e espaços).", "Score 1 if the answer equals the expected answer (ignores case and spaces)."),
  contains: t("Nota 1 se a resposta contiver o texto abaixo (ou a resposta esperada, se ficar vazio).", "Score 1 if the answer contains the text below (or the expected answer, if left empty)."),
  regex: t("Nota 1 se a expressão regular encontrar algo na resposta.", "Score 1 if the regular expression finds a match in the answer."),
};

type Props = {
  value: Omit<Evaluator, "id"> & { id?: string };
  onSave: (ev: Omit<Evaluator, "id"> & { id?: string }) => Promise<void>;
  onCancel?: () => void;
};

export function EvaluatorForm({ value, onSave, onCancel }: Props) {
  const [ev, setEv] = useState(value);
  const [saving, setSaving] = useState(false);
  const settings = useLoad(() => api.settings(), []);
  const providers = settings.data?.providers ?? [];
  const defaults = settings.data?.config.defaults;

  useEffect(() => setEv(value), [value]);

  // New judges default to the judge model from bancada.yaml.
  useEffect(() => {
    if (defaults && (ev.kind === "ragas" || ev.kind === "llm_judge") && !ev.judge_provider) {
      setEv((e) => ({ ...e, judge_provider: defaults.judge_provider, judge_model: defaults.judge_model }));
    }
  }, [defaults, ev.kind, ev.judge_provider]);

  const set = (patch: Partial<Evaluator>) => setEv((e) => ({ ...e, ...patch }));
  const needsJudge = ev.kind === "ragas" || ev.kind === "llm_judge";
  const needsEmbedding = ev.kind === "embedding" || (ev.kind === "ragas" && ev.metric === "answer_relevancy");
  const metrics =
    ev.kind === "ragas" ? ["faithfulness", "answer_relevancy", "context_precision"] : ev.kind === "rule" ? ["exact", "contains", "regex"] : [];

  async function save() {
    setSaving(true);
    try {
      await onSave(ev);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="col-flex" style={{ gap: 16 }}>
      <div className="row" style={{ alignItems: "flex-start" }}>
        <div className="field" style={{ flex: "2 1 220px" }}>
          <label htmlFor="ev-name">{t("Nome", "Name")}</label>
          <input id="ev-name" className="inp" value={ev.name} onChange={(e) => set({ name: e.target.value })} />
        </div>
        <div className="field" style={{ flex: "1 1 160px" }}>
          <label htmlFor="ev-kind">{t("Tipo", "Type")}</label>
          <select
            id="ev-kind"
            className="inp"
            value={ev.kind}
            disabled={!!ev.id}
            onChange={(e) => setEv({ ...evaluatorTemplates[e.target.value as EvaluatorKind], name: ev.name })}
          >
            {Object.entries(evaluatorKindLabel).map(([k, label]) => (
              <option key={k} value={k}>
                {label}
              </option>
            ))}
          </select>
        </div>
      </div>

      <div className="field">
        <label htmlFor="ev-desc">{t("Descrição", "Description")}</label>
        <input id="ev-desc" className="inp" value={ev.description} onChange={(e) => set({ description: e.target.value })} />
      </div>

      {metrics.length > 0 && (
        <div className="field">
          <label htmlFor="ev-metric">{ev.kind === "rule" ? t("Regra", "Rule") : t("Métrica", "Metric")}</label>
          <select id="ev-metric" className="inp" value={ev.metric} onChange={(e) => set({ metric: e.target.value })}>
            {metrics.map((m) => (
              <option key={m} value={m}>
                {metricLabel[m]}
              </option>
            ))}
          </select>
          <span className="hint">{metricHelp[ev.metric]}</span>
        </div>
      )}

      {needsJudge && (
        <ModelPicker
          label={t("Modelo juiz", "Judge model")}
          providers={providers.map((p) => p.name)}
          provider={ev.judge_provider}
          model={ev.judge_model}
          onChange={(provider, model) => set({ judge_provider: provider, judge_model: model })}
        />
      )}
      {needsEmbedding && (
        <ModelPicker
          label={t("Modelo de embedding", "Embedding model")}
          providers={providers.filter((p) => p.kind !== "anthropic").map((p) => p.name)}
          provider={ev.embedding_provider}
          model={ev.embedding_model}
          onChange={(provider, model) => set({ embedding_provider: provider, embedding_model: model })}
          hint={t("Ex.: ", "E.g.: ") + "nomic-embed-text (Ollama), text-embedding-3-small (OpenAI), text-embedding-004 (Gemini)."}
        />
      )}

      {ev.kind === "llm_judge" && (
        <div className="field">
          <label htmlFor="ev-rubric">{t("Rubrica do juiz", "Judge rubric")}</label>
          <textarea id="ev-rubric" className="inp" rows={6} value={ev.rubric} onChange={(e) => set({ rubric: e.target.value })} />
          <span className="hint">{t("O juiz responde com uma nota de 0 a 1 e um motivo curto. A resposta esperada é enviada junto, se houver.", "The judge answers with a score from 0 to 1 and a short reason. The expected answer is sent along, if there is one.")}</span>
          <label className="tgl" style={{ marginTop: 8 }}>
            <input type="checkbox" checked={!!ev.send_images} onChange={() => set({ send_images: !ev.send_images })} />
            {t(
              "Enviar também as imagens do input ao juiz (só para modelos com visão, ex.: GPT-4o, Claude, Gemini, gemma3)",
              "Also send the input images to the judge (vision models only, e.g. GPT-4o, Claude, Gemini, gemma3)",
            )}
          </label>
        </div>
      )}

      {ev.kind === "decision" && <DecisionFields ev={ev} set={set} providers={providers} />}

      {ev.kind === "rule" && ev.metric !== "exact" && (
        <div className="field">
          <label htmlFor="ev-pattern">{ev.metric === "regex" ? t("Expressão regular", "Regular expression") : t("Texto que deve aparecer", "Text that must appear")}</label>
          <input id="ev-pattern" className="inp mono" value={ev.pattern} onChange={(e) => set({ pattern: e.target.value })}
            placeholder={ev.metric === "regex" ? t("(?i)prazo.*\\d+ meses", "(?i)term.*\\d+ months") : t("deixe vazio para usar a resposta esperada", "leave empty to use the expected answer")} />
        </div>
      )}

      <div className="field" style={{ maxWidth: 220 }}>
        <label htmlFor="ev-thr">{t("Limiar de aprovação", "Passing threshold")}</label>
        <input id="ev-thr" className="inp" type="number" min={0} max={1} step={0.05} value={ev.threshold}
          onChange={(e) => set({ threshold: Number(e.target.value) })} />
      </div>

      <div className="row" style={{ justifyContent: "flex-end" }}>
        {onCancel && (
          <button className="btn" onClick={onCancel}>
            {t("Cancelar", "Cancel")}
          </button>
        )}
        <button className="btn pri" onClick={save} disabled={saving || !ev.name.trim()}>
          {saving ? t("Salvando…", "Saving…") : t("Salvar avaliador", "Save evaluator")}
        </button>
      </div>
    </div>
  );
}

function ModelPicker(props: {
  label: string;
  providers: string[];
  provider: string;
  model: string;
  hint?: string;
  onChange: (provider: string, model: string) => void;
}) {
  const id = props.label.replace(/\W/g, "");
  return (
    <div className="row" style={{ alignItems: "flex-start" }}>
      <div className="field" style={{ flex: "1 1 160px" }}>
        <label htmlFor={`${id}-p`}>{props.label}: {t("provedor", "provider")}</label>
        <select id={`${id}-p`} className="inp" value={props.provider} onChange={(e) => props.onChange(e.target.value, props.model)}>
          <option value="">{t("— escolha —", "— choose —")}</option>
          {props.providers.map((p) => (
            <option key={p}>{p}</option>
          ))}
        </select>
      </div>
      <div className="field" style={{ flex: "2 1 220px" }}>
        <label htmlFor={`${id}-m`}>{props.label}: {t("nome", "name")}</label>
        <input id={`${id}-m`} className="inp mono" value={props.model} onChange={(e) => props.onChange(props.provider, e.target.value)} />
        {props.hint && <span className="hint">{props.hint}</span>}
      </div>
    </div>
  );
}
