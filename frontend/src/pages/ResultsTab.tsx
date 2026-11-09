// Experiment › Resultados: run selector, KPIs, model comparison and answers per input.

import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, imageURL, type Experiment, type Result, type Score } from "../api";
import { useToast } from "../components/Toast";
import { fmtAgo, fmtCompact, fmtInt, fmtScore, fmtSeconds, fmtUSD, runStatus } from "../format";
import { useInterval, useLoad } from "../hooks";
import { t } from "../i18n";

type AnswerFilter = "all" | "errors" | "low";

export default function ResultsTab({ exp, onRun }: { exp: Experiment; onRun: () => void }) {
  const toast = useToast();
  const [params, setParams] = useSearchParams();
  const runs = useLoad(() => api.runs(exp.id), [exp.id]);
  const runId = Number(params.get("execucao")) || runs.data?.[0]?.id || 0;

  const results = useLoad(() => (runId ? api.runResults(runId) : Promise.resolve(null)), [runId]);
  const running = results.data?.run.status === "running";

  // While a run is in progress, refresh every 1.5 s.
  useInterval(
    () => {
      results.reload();
      runs.reload();
    },
    1500,
    running,
  );

  if (runs.data && runs.data.length === 0) {
    return (
      <section className="card empty">
        <h2>{t("Nenhuma execução ainda", "No runs yet")}</h2>
        <p className="muted">{t("Configure modelos, prompt e inputs, e então rode o experimento.", "Set up models, prompt and inputs, then run the experiment.")}</p>
        <button className="btn pri" onClick={onRun}>
          {t("Rodar experimento", "Run experiment")}
        </button>
      </section>
    );
  }
  if (results.error) return <div className="banner err">{results.error}</div>;
  if (!results.data) return <p className="muted">{t("Carregando…", "Loading…")}</p>;

  const { run, summary } = results.data;
  const status = runStatus(run);
  const progress = run.total ? Math.round((run.done / run.total) * 100) : 0;
  const maxP95 = Math.max(1, ...summary.models.map((m) => m.latency_p95_ms));

  return (
    <>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <div className="row">
          <div className="field" style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
            <label htmlFor="run">{t("Execução", "Run")}</label>
            <select
              id="run"
              className="inp"
              style={{ width: "auto", minWidth: 220 }}
              value={runId}
              onChange={(e) => setParams({ execucao: e.target.value })}
            >
              {(runs.data ?? []).map((r) => (
                <option key={r.id} value={r.id}>
                  #{r.id} · {fmtAgo(r.started_at)} · {fmtInt(r.total)} {t("chamadas", "calls")}
                </option>
              ))}
            </select>
          </div>
          <span className={`pill ${status.cls}`}>{status.label}</span>
        </div>
        <div className="row" style={{ gap: 10 }}>
          {running ? (
            <button
              className="btn danger"
              onClick={async () => {
                await api.cancelRun(run.id);
                toast(t("Execução cancelada.", "Run canceled."));
                results.reload();
              }}
            >
              {t("Cancelar execução", "Cancel run")}
            </button>
          ) : (
            <a className="btn" href={api.exportURL(run.id)}>
              {t("Exportar CSV", "Export CSV")}
            </a>
          )}
          <button className="btn pri" onClick={onRun} disabled={running}>
            {t("Rodar de novo", "Run again")}
          </button>
        </div>
      </div>

      {running && (
        <div className="card sec" style={{ gap: 8 }}>
          <div className="row" style={{ justifyContent: "space-between" }}>
            <strong>
              {t(`Rodando… ${fmtInt(run.done)} de ${fmtInt(run.total)} chamadas`, `Running… ${fmtInt(run.done)} of ${fmtInt(run.total)} calls`)}
            </strong>
            <span className="mono">{progress}%</span>
          </div>
          <div className="bar thick">
            <i style={{ width: `${progress}%` }} />
          </div>
        </div>
      )}

      <section className="kpis" aria-label={t("Resumo da execução", "Run summary")}>
        <div className="card kpi">
          <span className="l">{t("Chamadas", "Calls")}</span>
          <span className="v">{fmtInt(summary.calls)}</span>
          <span className="l">{summary.errors ? t(`${summary.errors} com erro`, `${summary.errors} with errors`) : t("nenhum erro", "no errors")}</span>
        </div>
        <div className="card kpi">
          <span className="l">{t("Tokens (entrada / saída)", "Tokens (input / output)")}</span>
          <span className="v">
            {fmtCompact(summary.input_tokens)} / {fmtCompact(summary.output_tokens)}
          </span>
          <span className="l">{t(`${fmtCompact(summary.input_tokens + summary.output_tokens)} no total`, `${fmtCompact(summary.input_tokens + summary.output_tokens)} total`)}</span>
        </div>
        <div className="card kpi">
          <span className="l">{t("Latência mediana", "Median latency")}</span>
          <span className="v">{fmtSeconds(summary.latency_p50_ms)}</span>
          <span className="l">p95: {fmtSeconds(summary.latency_p95_ms)}</span>
        </div>
        <div className="card kpi">
          <span className="l">{t("Custo em APIs", "API cost")}</span>
          <span className="v">{fmtUSD(summary.cost_usd)}</span>
          <span className="l">{summary.cost_complete ? t("modelos locais não contam", "local models are free") : t("alguns modelos sem preço configurado", "some models have no price configured")}</span>
        </div>
      </section>

      <section className="card sec">
        <div className="sechead">
          <h2>{t("Comparação entre modelos", "Model comparison")}</h2>
          <span className="muted">{t("Nota média vem dos avaliadores (aba Avaliação)", "Mean score comes from the evaluators (Evaluation tab)")}</span>
        </div>
        <div className="scroll-x">
          <div style={{ minWidth: 940 }}>
            <div className="grid-h" style={{ gridTemplateColumns: cmpColumns }}>
              <span>{t("Modelo", "Model")}</span>
              <span className="num">{t("Chamadas", "Calls")}</span>
              <span className="num">Tokens in</span>
              <span className="num">Tokens out</span>
              <span>{t("Latência p50 · p95", "Latency p50 · p95")}</span>
              <span className="num">{t("Custo", "Cost")}</span>
              <span className="num">{t("Erros", "Errors")}</span>
              <span className="num">{t("Nota", "Score")}</span>
            </div>
            {summary.models.map((m) => (
              <div key={m.provider + m.model} className="grid-r" style={{ gridTemplateColumns: cmpColumns, minHeight: 60 }}>
                <div style={{ minWidth: 0 }}>
                  <div className="mono cut" style={{ fontWeight: 500 }}>{m.model}</div>
                  <div className="muted tiny">{m.provider}</div>
                </div>
                <span className="mono num">{fmtInt(m.calls)}</span>
                <span className="mono num">{fmtCompact(m.input_tokens)}</span>
                <span className="mono num">{fmtCompact(m.output_tokens)}</span>
                <div>
                  <span className="mono">
                    {fmtSeconds(m.latency_p50_ms)} · {fmtSeconds(m.latency_p95_ms)}
                  </span>
                  <div className="bar" aria-hidden="true">
                    <i style={{ width: `${(m.latency_p95_ms / maxP95) * 100}%` }} />
                  </div>
                </div>
                <span className="mono num">{fmtUSD(m.cost_usd)}</span>
                <span className="mono num">{m.errors}</span>
                <span className="mono num" style={{ fontWeight: 600 }}>{fmtScore(m.mean_score)}</span>
              </div>
            ))}
          </div>
        </div>
      </section>

      <AnswersSection results={results.data.results} scores={results.data.scores} />
    </>
  );
}

const cmpColumns = "minmax(170px,1.4fr) 80px 90px 90px minmax(170px,1.3fr) 100px 70px 80px";

function AnswersSection({ results, scores }: { results: Result[]; scores: Score[] }) {
  const [filter, setFilter] = useState<AnswerFilter>("all");
  const [current, setCurrent] = useState(0);

  // Scores of each answer, and the mean per answer.
  const scoresByResult = useMemo(() => {
    const map = new Map<number, Score[]>();
    for (const s of scores) map.set(s.result_id, [...(map.get(s.result_id) ?? []), s]);
    return map;
  }, [scores]);
  const meanOf = (id: number) => {
    const values = (scoresByResult.get(id) ?? []).map((s) => s.score).filter((v): v is number => v != null);
    return values.length ? values.reduce((a, b) => a + b, 0) / values.length : null;
  };

  // Group answers by input.
  const inputs = useMemo(() => {
    const map = new Map<number, Result[]>();
    for (const r of results) map.set(r.input_index, [...(map.get(r.input_index) ?? []), r]);
    return [...map.entries()].sort((a, b) => a[0] - b[0]);
  }, [results]);

  const visible = inputs.filter(([, answers]) => {
    if (filter === "errors") return answers.some((a) => a.error);
    if (filter === "low") return answers.some((a) => (meanOf(a.id) ?? 1) < 0.7);
    return true;
  });

  useEffect(() => {
    if (visible.length && !visible.some(([i]) => i === current)) setCurrent(visible[0][0]);
  }, [visible, current]);

  const answers = inputs.find(([i]) => i === current)?.[1] ?? [];

  function inputNote(list: Result[]) {
    const errors = list.filter((a) => a.error).length;
    if (errors) return t(`${errors} erro(s)`, `${errors} error(s)`);
    const means = list.map((a) => meanOf(a.id)).filter((v): v is number => v != null);
    return means.length
      ? t(`nota mín. ${fmtScore(Math.min(...means))}`, `min. score ${fmtScore(Math.min(...means))}`)
      : t(`${list.length} respostas`, `${list.length} answers`);
  }

  return (
    <section className="card sec">
      <div className="sechead">
        <h2>{t("Respostas por input", "Answers per input")}</h2>
        <div className="row" style={{ gap: 8 }}>
          {(
            [
              ["all", t("Todos os inputs", "All inputs")],
              ["errors", t("Só com erro", "Only errors")],
              ["low", t("Nota abaixo de 0,7", "Score below 0.7")],
            ] as [AnswerFilter, string][]
          ).map(([k, label]) => (
            <button key={k} className="chipbtn" aria-pressed={filter === k} onClick={() => setFilter(k)}>
              {label}
            </button>
          ))}
        </div>
      </div>
      {visible.length === 0 ? (
        <p className="muted">{t("Nenhum input neste filtro.", "No inputs in this filter.")}</p>
      ) : (
        <div className="row" style={{ alignItems: "flex-start", gap: 24 }}>
          <div className="col-flex" style={{ flex: "0 0 220px", maxWidth: "100%", gap: 4, maxHeight: 620, overflowY: "auto" }}>
            {visible.map(([index, list]) => (
              <button key={index} className="brow" aria-current={index === current} onClick={() => setCurrent(index)}>
                <span style={{ fontWeight: 500 }}>Input {index + 1}</span>
                <span className="muted tiny">{inputNote(list)}</span>
              </button>
            ))}
          </div>
          <div className="col-flex" style={{ flex: "1 1 520px", minWidth: 0, gap: 16 }}>
            <div className="inbox">
              <strong>Input {current + 1}.</strong> {answers[0]?.user_prompt}
              {(answers[0]?.images ?? []).length > 0 && (
                <div className="row" style={{ marginTop: 10 }}>
                  {answers[0].images.map((ref) => (
                    <a key={ref} href={imageURL(ref)} target="_blank" rel="noreferrer">
                      <img src={imageURL(ref)} alt={ref} title={ref} className="thumb lg" />
                    </a>
                  ))}
                </div>
              )}
            </div>
            {answers[0]?.reference && (
              <div className="inbox">
                <strong>{t("Resposta esperada.", "Expected answer.")}</strong> {answers[0].reference}
              </div>
            )}
            <div className="rgrid">
              {answers.map((a) => (
                <AnswerCard key={a.id} answer={a} scores={scoresByResult.get(a.id) ?? []} mean={meanOf(a.id)} />
              ))}
            </div>
          </div>
        </div>
      )}
    </section>
  );
}

function AnswerCard({ answer: a, scores, mean }: { answer: Result; scores: Score[]; mean: number | null }) {
  let pill = { label: t("Sem nota", "No score"), cls: "draft" };
  if (a.error) pill = { label: t("Erro", "Error"), cls: "err" };
  else if (mean != null) pill = { label: t(`Nota ${fmtScore(mean)}`, `Score ${fmtScore(mean)}`), cls: mean >= 0.7 ? "ok" : "warn" };

  return (
    <article className="rcard">
      <header className="row" style={{ justifyContent: "space-between", gap: 8 }}>
        <span className="mono cut" style={{ fontWeight: 500 }}>
          {a.model}
          {a.repetition > 1 && <span className="muted"> · {t("rep.", "rep.")} {a.repetition}</span>}
        </span>
        <span className={`pill ${pill.cls}`}>{pill.label}</span>
      </header>
      <p className="txt">{a.error ? a.error : a.output || <span className="muted">{t("(resposta vazia)", "(empty answer)")}</span>}</p>
      <div className="stats mono">
        <span>{fmtSeconds(a.latency_ms)}</span>
        <span>
          {fmtInt(a.input_tokens)} in · {fmtInt(a.output_tokens)} out
        </span>
        <span>{fmtUSD(a.cost_usd)}</span>
      </div>
      {scores.length > 0 && (
        <div className="col-flex" style={{ gap: 4 }}>
          {scores.map((s) => (
            <div key={s.evaluator_id} className="muted tiny" title={s.reason || s.error}>
              <strong>{s.evaluator_id}</strong>: {s.score != null ? fmtScore(s.score) : t("erro", "error")}
              {(s.reason || s.error) && <> — {s.reason || s.error}</>}
            </div>
          ))}
        </div>
      )}
    </article>
  );
}
