// Experiment › Avaliação: choose evaluators, score a run, see the model × evaluator matrix.

import { useState } from "react";
import { Link } from "react-router-dom";
import { api, type Evaluator, type Experiment } from "../api";
import { EvaluatorForm } from "../components/EvaluatorForm";
import { useToast } from "../components/Toast";
import { evaluatorKindLabel, fmtAgo, fmtInt, fmtScore, metricLabel } from "../format";
import { useInterval, useLoad } from "../hooks";
import { t } from "../i18n";

export default function EvaluationTab({ exp, update }: { exp: Experiment; update: (e: Experiment) => void }) {
  const toast = useToast();
  // Reload the list when the experiment's columns change: "problems" depend on them.
  const evaluators = useLoad(() => api.evaluators(exp.id), [exp.id, exp.dataset.reference_column, exp.dataset.context_column]);
  const health = useLoad(() => api.health(), []);
  const runs = useLoad(() => api.runs(exp.id), [exp.id]);
  const [runId, setRunId] = useState(0);
  const [selected, setSelected] = useState("");

  const currentRunId = runId || runs.data?.find((r) => r.status !== "running")?.id || 0;
  const evaluation = useLoad(() => (currentRunId ? api.runEvaluation(currentRunId) : Promise.resolve(null)), [currentRunId]);
  const progress = evaluation.data?.progress;
  useInterval(evaluation.reload, 1500, !!progress?.running);

  const all = evaluators.data ?? [];
  const active = all.filter((e) => exp.evaluators.includes(e.id));
  const current = all.find((e) => e.id === selected) ?? all[0];

  function toggle(ev: Evaluator) {
    const ids = exp.evaluators.includes(ev.id) ? exp.evaluators.filter((x) => x !== ev.id) : [...exp.evaluators, ev.id];
    update({ ...exp, evaluators: ids });
  }

  async function evaluate() {
    if (!currentRunId) return;
    try {
      await api.evaluateRun(currentRunId, exp.evaluators);
      toast(t("Avaliação iniciada.", "Evaluation started."));
      evaluation.reload();
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }

  const matrix = evaluation.data?.matrix ?? {};
  const gridColumns = `minmax(170px,1.4fr) repeat(${active.length}, minmax(100px,1fr)) 100px`;
  const usesModels = active.some((e) => e.kind !== "rule");

  return (
    <>
      {health.data && !health.data.evaluator && (
        <div className="banner warn">
          <span>
            {t(
              "O serviço de avaliação (Python) está offline. Avaliadores do tipo Motor de decisão funcionam mesmo assim. Para os outros, feche o Bancada e abra de novo com ./start.sh, que instala o que falta.",
              "The evaluation service (Python) is offline. Decision engine evaluators still work. For the others, close Bancada and open it again with ./start.sh, which installs what is missing.",
            )}
          </span>
        </div>
      )}

      <section className="card row" style={{ padding: "16px 24px", justifyContent: "space-between", gap: 16 }}>
        <div className="col-flex" style={{ gap: 4 }}>
          <div className="row" style={{ gap: 8 }}>
            <label htmlFor="ev-run" style={{ fontWeight: 600 }}>
              {t("Execução", "Run")}
            </label>
            <select
              id="ev-run"
              className="inp"
              style={{ width: "auto", minWidth: 200 }}
              value={currentRunId}
              onChange={(e) => setRunId(Number(e.target.value))}
            >
              {(runs.data ?? []).length === 0 && <option value={0}>{t("nenhuma execução ainda", "no runs yet")}</option>}
              {(runs.data ?? []).map((r) => (
                <option key={r.id} value={r.id}>
                  #{r.id} · {fmtAgo(r.started_at)}
                </option>
              ))}
            </select>
            <span className="muted">
              {fmtInt(evaluation.data?.answers ?? 0)} {t("respostas", "answers")} · {active.length} {t("avaliadores ativos", "active evaluators")}
            </span>
          </div>
          {usesModels && <span className="muted small">{t("O juiz também gasta tokens da sua conta do provedor (cerca de 1 a 3 chamadas por resposta).", "The judge also spends tokens from your provider account (about 1 to 3 calls per answer).")}</span>}
        </div>
        <button className="btn pri" onClick={evaluate} disabled={!currentRunId || active.length === 0 || progress?.running}>
          {progress?.running ? t(`Avaliando… ${progress.done}/${progress.total}`, `Evaluating… ${progress.done}/${progress.total}`) : t("Avaliar execução", "Evaluate run")}
        </button>
      </section>

      <div className="row" style={{ alignItems: "flex-start", gap: 24 }}>
        <section className="card sec" style={{ flex: "0 1 400px", minWidth: 0, width: "100%" }}>
          <div className="sechead">
            <h2>{t("Avaliadores", "Evaluators")}</h2>
            <span className="muted">
              {t(`${active.length} de ${all.length} ativos`, `${active.length} of ${all.length} active`)}
            </span>
          </div>
          {all.length === 0 && (
            <p className="muted">
              {t("Nenhum avaliador ainda. ", "No evaluators yet. ")}
              <Link to="/avaliadores" className="btn link">{t("Crie um em Avaliadores", "Create one in Evaluators")}</Link>
            </p>
          )}
          <div className="col-flex" style={{ gap: 8 }}>
            {all.map((ev) => {
              const on = exp.evaluators.includes(ev.id);
              return (
                <div key={ev.id} className={`ev ${on ? "" : "off"} ${current?.id === ev.id ? "sel" : ""}`}>
                  <label className="tgl">
                    <input type="checkbox" aria-label={t(`Ativar ${ev.name}`, `Enable ${ev.name}`)} checked={on} onChange={() => toggle(ev)} />
                  </label>
                  <button className="evbtn" aria-pressed={current?.id === ev.id} onClick={() => setSelected(ev.id)}>
                    <span style={{ fontWeight: 500 }}>{ev.name}</span>
                    <span className="row" style={{ gap: 6 }}>
                      <span className="pill draft">{evaluatorKindLabel[ev.kind]}</span>
                      {(ev.problems ?? []).length > 0 && <span className="pill warn">{ev.problems![0]}</span>}
                    </span>
                  </button>
                </div>
              );
            })}
          </div>
          <Link to="/avaliadores" className="btn sm">
            {t("Criar ou gerenciar avaliadores", "Create or manage evaluators")}
          </Link>
        </section>

        {current && (
          <section className="card sec" style={{ flex: "1 1 460px", minWidth: 0 }}>
            <div className="sechead">
              <h2>{current.name}</h2>
              <span className="pill run">
                {evaluatorKindLabel[current.kind]}
                {current.metric ? ` · ${metricLabel[current.metric] ?? current.metric}` : ""}
              </span>
            </div>
            {(current.problems ?? []).map((p) => (
              <div key={p} className="banner warn small">
                {p}{" "}
                {(p.includes("coluna") || p.includes("column")) && (
                  <>
                    {t("— mapeie na aba ", "— map it in the ")}
                    <Link to={`/experimentos/${exp.id}`} className="btn link">{t("Configuração", "Setup")}</Link>
                    {t("", " tab")}
                  </>
                )}
              </div>
            ))}
            <EvaluatorForm
              value={current}
              onSave={async (ev) => {
                try {
                  await api.saveEvaluator(ev as Evaluator);
                  toast(t("Avaliador salvo.", "Evaluator saved."));
                  evaluators.reload();
                } catch (e) {
                  toast((e as Error).message, "err");
                }
              }}
            />
            <div className="row" style={{ justifyContent: "space-between" }}>
              <span className="muted mono tiny">evaluators/{current.id}.yaml · {t("compartilhado entre experimentos", "shared between experiments")}</span>
              <Link to={`/avaliadores?avaliador=${current.id}`} className="btn sm">{t("Testar com uma planilha", "Test with a spreadsheet")}</Link>
            </div>
          </section>
        )}
      </div>

      <section className="card sec">
        <div className="sechead">
          <h2>{t("Nota por modelo e avaliador", "Score by model and evaluator")}</h2>
          <span className="muted">{t("Execução", "Run")} #{currentRunId || "—"} · {t("de 0 a 1, maior é melhor", "from 0 to 1, higher is better")}</span>
        </div>
        {active.length === 0 || (evaluation.data?.models ?? []).length === 0 ? (
          <p className="muted">{t("Ative avaliadores e clique em “Avaliar execução” para ver as notas.", "Enable evaluators and click “Evaluate run” to see the scores.")}</p>
        ) : (
          <div className="scroll-x">
            <div style={{ minWidth: 720 }}>
              <div className="grid-h" style={{ gridTemplateColumns: gridColumns, gap: 12 }}>
                <span>{t("Modelo", "Model")}</span>
                {active.map((ev) => (
                  <span key={ev.id} style={{ textAlign: "center" }}>
                    {ev.name}
                  </span>
                ))}
                <span style={{ textAlign: "center" }}>{t("Média", "Mean")}</span>
              </div>
              {evaluation.data!.models.map((model) => {
                const means = active.map((ev) => matrix[model]?.[ev.id]?.mean ?? null);
                const valid = means.filter((m): m is number => m != null);
                const avg = valid.length ? valid.reduce((a, b) => a + b, 0) / valid.length : null;
                return (
                  <div key={model} className="grid-r" style={{ gridTemplateColumns: gridColumns, gap: 12, minHeight: 60 }}>
                    <span className="mono cut" style={{ fontWeight: 500 }}>{model}</span>
                    {active.map((ev, i) => {
                      const cell = matrix[model]?.[ev.id];
                      return (
                        <span key={ev.id} className="cell" style={cellStyle(means[i])}
                          title={cell?.errors ? t(`${cell.errors} erro(s) na avaliação`, `${cell.errors} evaluation error(s)`) : ""}>
                          {fmtScore(means[i])}
                          {cell?.errors ? " *" : ""}
                        </span>
                      );
                    })}
                    <span className="cell" style={{ fontWeight: 600, border: "1px solid var(--line)" }}>{fmtScore(avg)}</span>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </section>
    </>
  );
}

// Darker teal = higher score (same scale as the design).
function cellStyle(v: number | null) {
  if (v == null) return { color: "var(--muted)" };
  const alpha = Math.max(0.06, Math.min(0.5, 0.08 + (v - 0.5) * 0.9));
  return { background: `rgba(11,110,115,${alpha.toFixed(2)})` };
}
