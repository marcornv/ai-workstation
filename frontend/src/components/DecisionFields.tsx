// The "Motor de decisão" part of the evaluator form: which decision model,
// the question (yes/no, choice or levels), the text it reads, and a quick test.

import { useState } from "react";
import { api, type DecisionOption, type DecisionQuestion, type DecisionTestResult, type Evaluator, type ProviderView } from "../api";
import { fmtScore } from "../format";
import { useLoad } from "../hooks";
import { t } from "../i18n";

type Draft = Omit<Evaluator, "id"> & { id?: string };

// Must match defaultStateTemplate in backend/decisions.go.
const defaultStateTemplate = "Question:\n{question}\n\nModel answer:\n{answer}\n\nExpected answer:\n{reference}";

export const emptyQuestion: DecisionQuestion = { type: "noul", instructions: "", true_means: "", false_means: "", options: [] };

export function DecisionFields({ ev, set, providers }: { ev: Draft; set: (patch: Partial<Evaluator>) => void; providers: ProviderView[] }) {
  const q = { ...emptyQuestion, ...ev.question, options: ev.question?.options ?? [] };
  const setQuestion = (patch: Partial<DecisionQuestion>) => set({ question: { ...q, ...patch } });
  const decisionProviders = providers.filter((p) => p.kind === "ollama" || p.kind === "typesafe");
  const provider = providers.find((p) => p.name === ev.decision_provider);
  const models = useLoad(
    () => (ev.decision_provider ? api.providerModels(ev.decision_provider) : Promise.resolve({ models: [] })),
    [ev.decision_provider],
  );

  function setOption(i: number, patch: Partial<DecisionOption>) {
    setQuestion({ options: q.options.map((o, j) => (j === i ? { ...o, ...patch } : o)) });
  }
  function changeType(type: DecisionQuestion["type"]) {
    const options =
      type === "choice"
        ? [
            { label: t("correta", "correct"), description: t("A resposta está correta", "The answer is correct"), pass: true },
            { label: t("incorreta", "incorrect"), description: t("A resposta está errada ou incompleta", "The answer is wrong or incomplete"), pass: false },
          ]
        : type === "score"
          ? [
              { label: "", description: "Poor", pass: false },
              { label: "", description: "Acceptable", pass: false },
              { label: "", description: "Excellent", pass: false },
            ]
          : [];
    setQuestion({ type, options });
  }

  return (
    <div className="col-flex" style={{ gap: 16 }}>
      <div className="row" style={{ alignItems: "flex-start" }}>
        <div className="field" style={{ flex: "1 1 160px" }}>
          <label htmlFor="dec-provider">{t("Provedor", "Provider")}</label>
          <select id="dec-provider" className="inp" value={ev.decision_provider ?? ""} onChange={(e) => set({ decision_provider: e.target.value })}>
            <option value="">{t("— escolha —", "— choose —")}</option>
            {decisionProviders.map((p) => (
              <option key={p.name}>{p.name}</option>
            ))}
          </select>
        </div>
        <div className="field" style={{ flex: "2 1 220px" }}>
          <label htmlFor="dec-model">{t("Modelo de decisão", "Decision model")}</label>
          <input id="dec-model" className="inp mono" list="dec-models" value={ev.decision_model ?? ""}
            placeholder={provider?.kind === "typesafe" ? "jev-latest" : "laya"} onChange={(e) => set({ decision_model: e.target.value })} />
          <datalist id="dec-models">
            {(models.data?.models ?? []).map((m) => (
              <option key={m} value={m} />
            ))}
          </datalist>
          <span className="hint">
            {t(
              "No Ollama: laya (rápido, só inglês, lê até ~512 tokens) ou autotrust-jev (textos longos; :35b também vê imagens). No TypeSafe: jev-latest.",
              "On Ollama: laya (fast, English only, reads up to ~512 tokens) or autotrust-jev (long texts; :35b also sees images). On TypeSafe: jev-latest.",
            )}
          </span>
        </div>
      </div>

      <div className="field">
        <label htmlFor="dec-type">{t("Tipo de pergunta", "Question type")}</label>
        <select id="dec-type" className="inp" value={q.type} onChange={(e) => changeType(e.target.value as DecisionQuestion["type"])}>
          <option value="noul">{t("Sim ou não (nota = probabilidade de “sim”)", "Yes or no (score = probability of “yes”)")}</option>
          <option value="choice">{t("Escolha entre opções (nota = soma das opções aprovadas)", "Choice between options (score = sum of passing options)")}</option>
          <option value="score">{t("Nota em níveis (nota = nível esperado ÷ nível máximo)", "Rating levels (score = expected level ÷ top level)")}</option>
        </select>
      </div>

      <div className="field">
        <label htmlFor="dec-instr">{t("Pergunta", "Question")}</label>
        <textarea id="dec-instr" className="inp" rows={2} value={q.instructions}
          placeholder="Is the model answer correct according to the expected answer?"
          onChange={(e) => setQuestion({ instructions: e.target.value })} />
        <span className="hint">{t("Seja específico. O Laya só entende inglês: escreva a pergunta e as opções em inglês para ele.", "Be specific. Laya only understands English: write the question and options in English for it.")}</span>
      </div>

      {q.type === "noul" && (
        <div className="row" style={{ alignItems: "flex-start" }}>
          <div className="field" style={{ flex: "1 1 220px" }}>
            <label htmlFor="dec-true">{t("“Sim” significa", "“Yes” means")}</label>
            <input id="dec-true" className="inp" value={q.true_means} onChange={(e) => setQuestion({ true_means: e.target.value })} />
          </div>
          <div className="field" style={{ flex: "1 1 220px" }}>
            <label htmlFor="dec-false">{t("“Não” significa", "“No” means")}</label>
            <input id="dec-false" className="inp" value={q.false_means} onChange={(e) => setQuestion({ false_means: e.target.value })} />
          </div>
        </div>
      )}

      {q.type === "choice" && (
        <div className="field">
          <span className="label">{t("Opções (2 a 26)", "Options (2 to 26)")}</span>
          {q.options.map((o, i) => (
            <div key={i} className="row" style={{ gap: 8, flexWrap: "nowrap" }}>
              <input className="inp mono" style={{ flex: "0 1 160px" }} aria-label={t(`Nome da opção ${i + 1}`, `Option ${i + 1} name`)} value={o.label}
                placeholder={t("nome", "name")} onChange={(e) => setOption(i, { label: e.target.value })} />
              <input className="inp" style={{ flex: "1 1 auto" }} aria-label={t(`Descrição da opção ${i + 1}`, `Option ${i + 1} description`)} value={o.description}
                placeholder={t("o que esta opção significa", "what this option means")} onChange={(e) => setOption(i, { description: e.target.value })} />
              <label className="tgl" style={{ whiteSpace: "nowrap" }}>
                <input type="checkbox" checked={o.pass} onChange={() => setOption(i, { pass: !o.pass })} /> {t("aprovada", "passing")}
              </label>
              <button className="btn sm danger" aria-label={t(`Remover opção ${i + 1}`, `Remove option ${i + 1}`)} disabled={q.options.length <= 2}
                onClick={() => setQuestion({ options: q.options.filter((_, j) => j !== i) })}>{t("Remover", "Remove")}</button>
            </div>
          ))}
          <div>
            <button className="btn sm" disabled={q.options.length >= 26}
              onClick={() => setQuestion({ options: [...q.options, { label: "", description: "", pass: false }] })}>
              {t("Adicionar opção", "Add option")}
            </button>
          </div>
          <span className="hint">{t("A nota é a soma das probabilidades das opções marcadas como “aprovada”.", "The score is the sum of the probabilities of the options marked as “passing”.")}</span>
        </div>
      )}

      {q.type === "score" && (
        <div className="field">
          <span className="label">{t("Níveis, do pior para o melhor (2 a 10)", "Levels, from worst to best (2 to 10)")}</span>
          {q.options.map((o, i) => (
            <div key={i} className="row" style={{ gap: 8, flexWrap: "nowrap" }}>
              <span className="mono muted" style={{ width: 20 }}>{i}</span>
              <input className="inp" style={{ flex: "1 1 auto" }} aria-label={t(`Nível ${i}`, `Level ${i}`)} value={o.description}
                onChange={(e) => setOption(i, { description: e.target.value })} />
              <button className="btn sm danger" aria-label={t(`Remover nível ${i}`, `Remove level ${i}`)} disabled={q.options.length <= 2}
                onClick={() => setQuestion({ options: q.options.filter((_, j) => j !== i) })}>{t("Remover", "Remove")}</button>
            </div>
          ))}
          <div>
            <button className="btn sm" disabled={q.options.length >= 10}
              onClick={() => setQuestion({ options: [...q.options, { label: "", description: "", pass: false }] })}>
              {t("Adicionar nível", "Add level")}
            </button>
          </div>
        </div>
      )}

      <div className="field">
        <label htmlFor="dec-state">{t("Texto que o modelo de decisão lê", "Text the decision model reads")}</label>
        <textarea id="dec-state" className="inp mono" rows={6} value={ev.state_template ?? ""} placeholder={defaultStateTemplate}
          onChange={(e) => set({ state_template: e.target.value })} />
        <span className="hint">
          {t("Vazio = o texto padrão acima. Use ", "Empty = the default text above. Use ")}
          <span className="mono">{"{question}"}</span> {t("(o prompt)", "(the prompt)")},{" "}
          <span className="mono">{"{answer}"}</span> {t("(a resposta)", "(the answer)")},{" "}
          <span className="mono">{"{reference}"}</span> {t("(resposta esperada) e", "(expected answer) and")}{" "}
          <span className="mono">{"{context}"}</span>.
        </span>
      </div>

      {provider?.kind === "ollama" && (
        <label className="tgl">
          <input type="checkbox" checked={!!ev.send_images} onChange={() => set({ send_images: !ev.send_images })} />
          {t("Enviar também as imagens do input (só para modelos com visão, ex.: autotrust-jev:35b)", "Also send the input images (vision models only, e.g. autotrust-jev:35b)")}
        </label>
      )}

      <DecisionTest ev={ev} />
    </div>
  );
}

// Runs the evaluator (even unsaved) on one example, to tune the question and threshold.
function DecisionTest({ ev }: { ev: Draft }) {
  const [item, setItem] = useState({ question: "", answer: "", reference: "" });
  const [result, setResult] = useState<DecisionTestResult | null>(null);
  const [error, setError] = useState("");
  const [testing, setTesting] = useState(false);

  async function run() {
    setTesting(true);
    setError("");
    setResult(null);
    try {
      setResult(await api.testEvaluator(ev, item));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setTesting(false);
    }
  }

  const probs = Object.entries(result?.answer.probabilities ?? {}).sort((a, b) => b[1] - a[1]);

  return (
    <div className="card soft col-flex" style={{ padding: 16, gap: 12 }}>
      <strong>{t("Testar com um exemplo", "Test with an example")}</strong>
      <div className="row" style={{ alignItems: "flex-start" }}>
        {(
          [
            ["question", t("Input (prompt)", "Input (prompt)")],
            ["answer", t("Resposta do modelo", "Model answer")],
            ["reference", t("Resposta esperada", "Expected answer")],
          ] as const
        ).map(([key, label]) => (
          <div key={key} className="field" style={{ flex: "1 1 200px" }}>
            <label htmlFor={`test-${key}`}>{label}</label>
            <textarea id={`test-${key}`} className="inp" rows={3} value={item[key]}
              onChange={(e) => setItem({ ...item, [key]: e.target.value })} />
          </div>
        ))}
      </div>
      <div>
        <button className="btn sm pri" onClick={run} disabled={testing || !item.answer.trim()}>
          {testing ? t("Perguntando ao modelo…", "Asking the model…") : t("Testar", "Test")}
        </button>
      </div>
      {error && <div className="banner err small">{error}</div>}
      {result && (
        <div className="col-flex" style={{ gap: 8 }}>
          <div className="row">
            <span className={`pill ${result.passed ? "ok" : "warn"}`}>
              {t("Nota", "Score")} {fmtScore(result.score)} · {result.passed ? t("aprovada", "passed") : t("abaixo do limiar", "below the threshold")}
            </span>
            <span className="small">{result.reason}</span>
          </div>
          {probs.map(([label, p]) => (
            <div key={label} className="row" style={{ gap: 8, flexWrap: "nowrap" }}>
              <span className="mono small cut" style={{ width: 160 }}>{result.answer.type === "score" ? t(`nível ${label}`, `level ${label}`) : label}</span>
              <span className="bar thick" style={{ flex: 1, marginTop: 0 }}><i style={{ width: `${p * 100}%` }} /></span>
              <span className="mono small" style={{ width: 44, textAlign: "right" }}>{fmtScore(p)}</span>
            </div>
          ))}
          <details>
            <summary className="small muted" style={{ cursor: "pointer" }}>{t("Ver o texto enviado ao modelo", "See the text sent to the model")}</summary>
            <pre className="prompt-box">{result.state}</pre>
          </details>
        </div>
      )}
    </div>
  );
}
