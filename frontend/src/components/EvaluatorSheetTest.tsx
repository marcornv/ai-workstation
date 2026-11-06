// "Testar com uma planilha": run a saved evaluator over every row of a
// spreadsheet, without an experiment. With a human-verdict column it shows how
// often the evaluator agrees with people, and suggests a better threshold.

import { useEffect, useRef, useState } from "react";
import { api, type Evaluator, type SheetColumns } from "../api";
import { useToast } from "./Toast";
import { fmtAgo, fmtInt, fmtScore } from "../format";
import { useInterval, useLoad } from "../hooks";
import { t } from "../i18n";

const emptyColumns: SheetColumns = { answer: "", question: "", reference: "", context: "", images: "", human: "" };

const pct = (v: number | null | undefined) => (v == null ? "—" : Math.round(v * 100) + "%");

type Props = { evaluator: Evaluator; onEvaluatorSaved: (ev: Evaluator) => void };

export function EvaluatorSheetTest({ evaluator, onEvaluatorSaved }: Props) {
  const tests = useLoad(() => api.evaluatorTests(evaluator.id), [evaluator.id]);
  const [testId, setTestId] = useState(0);
  const currentId = testId || tests.data?.[0]?.id || 0;

  return (
    <section className="card sec">
      <div className="sechead">
        <h2>{t("Testar com uma planilha", "Test with a spreadsheet")}</h2>
        <span className="muted small">{t("Sem precisar de um experimento", "No experiment needed")}</span>
      </div>
      <p className="muted small">
        {t(
          "Use uma planilha com respostas já conhecidas. Se ela tiver uma coluna com o seu veredito (sim/não, 1/0, aprovado/reprovado), o Bancada mostra quanto o avaliador concorda com você e qual limiar funcionaria melhor.",
          "Use a spreadsheet with answers you already know. If it has a column with your verdict (yes/no, 1/0, pass/fail), Bancada shows how often the evaluator agrees with you and which threshold would work best.",
        )}
      </p>

      <NewSheetTest
        evaluator={evaluator}
        onStarted={(id) => {
          setTestId(id);
          tests.reload();
        }}
      />

      {(tests.data ?? []).length > 0 && (
        <>
          <div className="row" style={{ borderTop: "1px solid var(--line2)", paddingTop: 16 }}>
            <label htmlFor="test-pick" style={{ fontWeight: 600 }}>
              {t("Testes anteriores", "Previous tests")}
            </label>
            <select id="test-pick" className="inp" style={{ width: "auto", minWidth: 260 }} value={currentId}
              onChange={(e) => setTestId(Number(e.target.value))}>
              {(tests.data ?? []).map((test) => (
                <option key={test.id} value={test.id}>
                  #{test.id} · {test.dataset} · {fmtAgo(test.started_at)}
                </option>
              ))}
            </select>
          </div>
          {currentId > 0 && (
            <TestResult
              key={currentId}
              testId={currentId}
              evaluator={evaluator}
              onChanged={tests.reload}
              onDeleted={() => {
                setTestId(0);
                tests.reload();
              }}
              onEvaluatorSaved={onEvaluatorSaved}
            />
          )}
        </>
      )}
    </section>
  );
}

// ---------- Choosing the spreadsheet and its columns ----------

// guessColumns maps spreadsheet column names to their role, by name.
function guessColumns(columns: string[], imageGuess: string[]): SheetColumns {
  const used: string[] = [];
  const find = (pattern: RegExp) => {
    const col = columns.find((c) => !used.includes(c) && pattern.test(c.toLowerCase()));
    if (col) used.push(col);
    return col ?? "";
  };
  const reference = find(/esperad|referen|expected|gabarito/);
  const human = find(/veredito|humano|human|verdict|label|r[oó]tulo|julgamento|aprovad|corret/);
  const images = imageGuess.find((c) => !used.includes(c)) ?? "";
  if (images) used.push(images);
  const context = find(/contexto|context/);
  const answer = find(/resposta|answer|output|sa[ií]da|resultado/);
  const question = find(/pergunta|question|input|prompt|entrada/);
  return { answer, question, reference, context, images, human };
}

function NewSheetTest({ evaluator, onStarted }: { evaluator: Evaluator; onStarted: (id: number) => void }) {
  const toast = useToast();
  const fileInput = useRef<HTMLInputElement>(null);
  const datasets = useLoad(() => api.datasets(), []);
  const [file, setFile] = useState("");
  const [columns, setColumns] = useState<SheetColumns>(emptyColumns);
  const [starting, setStarting] = useState(false);
  const preview = useLoad(() => (file ? api.previewDataset(file, 3) : Promise.resolve(null)), [file]);

  // When a spreadsheet is chosen, guess which column is which.
  useEffect(() => {
    if (preview.data) setColumns(guessColumns(preview.data.columns, preview.data.image_columns_guess));
    else setColumns(emptyColumns);
  }, [preview.data]);

  async function upload(f: File) {
    try {
      const res = await api.uploadDataset(f);
      await datasets.reload();
      setFile(res.file);
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }

  async function start() {
    setStarting(true);
    try {
      const { test_id } = await api.startEvaluatorTest(evaluator.id, file, columns);
      toast(t("Teste iniciado.", "Test started."));
      onStarted(test_id);
    } catch (e) {
      toast((e as Error).message, "err");
    } finally {
      setStarting(false);
    }
  }

  const cols = preview.data?.columns ?? [];
  const usesPaidModel =
    evaluator.kind === "llm_judge" || evaluator.kind === "ragas" || evaluator.kind === "embedding" || evaluator.kind === "decision";
  const roles: [keyof SheetColumns, string, string][] = [
    ["answer", t("Resposta a avaliar (obrigatória)", "Answer to evaluate (required)"), t("O texto que o avaliador julga.", "The text the evaluator judges.")],
    ["question", t("Input / pergunta", "Input / question"), t("Vira {question} no texto do avaliador.", "Becomes {question} in the evaluator's text.")],
    ["reference", t("Resposta esperada", "Expected answer"), t("Vira {reference}.", "Becomes {reference}.")],
    ["context", t("Contexto", "Context"), t("Vira {context}. Se vazio, usa o input.", "Becomes {context}. If empty, the input is used.")],
    ["images", t("Imagens", "Images"), t("Caminho dentro de data/ ou link.", "Path inside data/ or a link.")],
    ["human", t("Seu veredito (opcional)", "Your verdict (optional)"), t("sim/não, 1/0, aprovado/reprovado ou uma nota de 0 a 1.", "yes/no, 1/0, pass/fail or a score from 0 to 1.")],
  ];

  return (
    <div className="col-flex" style={{ gap: 14 }}>
      <div className="row" style={{ alignItems: "flex-end" }}>
        <div className="field" style={{ flex: "1 1 260px" }}>
          <label htmlFor="sheet-file">{t("Planilha", "Spreadsheet")}</label>
          <select id="sheet-file" className="inp" value={file} onChange={(e) => setFile(e.target.value)}>
            <option value="">{t("— escolha um arquivo —", "— choose a file —")}</option>
            {(datasets.data ?? []).map((f) => (
              <option key={f}>{f}</option>
            ))}
          </select>
        </div>
        <button className="btn" onClick={() => fileInput.current?.click()}>
          {t("Enviar CSV / JSONL", "Upload CSV / JSONL")}
        </button>
        <input ref={fileInput} type="file" accept=".csv,.jsonl,.ndjson" hidden
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) upload(f);
            e.target.value = "";
          }} />
      </div>

      {preview.error && <div className="banner err small">{preview.error}</div>}

      {preview.data && (
        <>
          <div className="row" style={{ alignItems: "flex-start" }}>
            {roles.map(([role, label, hint]) => (
              <div key={role} className="field" style={{ flex: "1 1 200px" }}>
                <label htmlFor={`col-${role}`}>{label}</label>
                <select id={`col-${role}`} className="inp" value={columns[role]}
                  onChange={(e) => setColumns({ ...columns, [role]: e.target.value })}>
                  <option value="">{t("— nenhuma —", "— none —")}</option>
                  {cols.map((c) => (
                    <option key={c}>{c}</option>
                  ))}
                </select>
                <span className="hint">{hint}</span>
              </div>
            ))}
          </div>
          <div className="row">
            <button className="btn pri" onClick={start} disabled={starting || !columns.answer}>
              {starting ? t("Iniciando…", "Starting…") : t(`Rodar avaliador em ${fmtInt(preview.data.count)} linhas`, `Run evaluator on ${fmtInt(preview.data.count)} rows`)}
            </button>
            {usesPaidModel && (
              <span className="hint">
                {t(
                  "Cada linha é uma chamada ao modelo do avaliador. Se ele for de uma API paga, cada linha é cobrada.",
                  "Each row is one call to the evaluator's model. If it is a paid API, each row is billed.",
                )}
              </span>
            )}
          </div>
        </>
      )}
    </div>
  );
}

// ---------- Showing one test ----------

type Filter = "all" | "disagree" | "errors";

function TestResult(props: {
  testId: number;
  evaluator: Evaluator;
  onChanged: () => void;
  onDeleted: () => void;
  onEvaluatorSaved: (ev: Evaluator) => void;
}) {
  const toast = useToast();
  const detail = useLoad(() => api.evaluatorTest(props.testId), [props.testId]);
  const [filter, setFilter] = useState<Filter>("all");
  const running = detail.data?.test.status === "running";

  useInterval(
    async () => {
      const next = await detail.reload();
      if (next && next.test.status !== "running") props.onChanged();
    },
    1500,
    running,
  );

  if (detail.error) return <div className="banner err">{detail.error}</div>;
  if (!detail.data) return <p className="muted">{t("Carregando…", "Loading…")}</p>;

  const { test, summary: s, rows, evaluator: snapshot, columns } = detail.data;
  const threshold = snapshot.threshold;
  const hasHuman = !!columns.human;
  const progress = test.total ? Math.round((test.done / test.total) * 100) : 0;

  const visible = rows.filter((r) => (filter === "errors" ? !!r.error : filter === "disagree" ? r.agrees === false : true));
  const rowColumns = hasHuman ? rowColumnsWithHuman : rowColumnsWithoutHuman;

  async function useBestThreshold() {
    if (s.best_threshold == null) return;
    try {
      const saved = await api.saveEvaluator({ ...props.evaluator, threshold: s.best_threshold });
      toast(t(`Limiar alterado para ${fmtScore(s.best_threshold)}. Vale para as próximas avaliações.`, `Threshold changed to ${fmtScore(s.best_threshold)}. It applies to future evaluations.`));
      props.onEvaluatorSaved(saved);
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }

  return (
    <div className="col-flex" style={{ gap: 16 }}>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <span className="muted small">
          {test.dataset} · {t("limiar usado:", "threshold used:")} <span className="mono">{fmtScore(threshold)}</span>
        </span>
        <div className="row" style={{ gap: 8 }}>
          {running ? (
            <button className="btn sm danger" onClick={async () => { await api.cancelEvaluatorTest(test.id); detail.reload(); }}>
              {t("Cancelar", "Cancel")}
            </button>
          ) : (
            <a className="btn sm" href={api.evaluatorTestExportURL(test.id)}>{t("Exportar CSV", "Export CSV")}</a>
          )}
          <button className="btn sm danger"
            onClick={async () => { await api.deleteEvaluatorTest(test.id); toast(t("Teste excluído.", "Test deleted.")); props.onDeleted(); }}>
            {t("Excluir teste", "Delete test")}
          </button>
        </div>
      </div>

      {running && (
        <div className="col-flex" style={{ gap: 6 }}>
          <strong>{t(`Avaliando… ${fmtInt(test.done)} de ${fmtInt(test.total)} linhas`, `Evaluating… ${fmtInt(test.done)} of ${fmtInt(test.total)} rows`)}</strong>
          <div className="bar thick"><i style={{ width: `${progress}%` }} /></div>
        </div>
      )}
      {test.status === "canceled" && <div className="banner warn small">{t("Teste cancelado: os números consideram só as linhas avaliadas.", "Test canceled: the numbers only count the rows that were evaluated.")}</div>}

      <div className="kpis">
        <div className="card kpi">
          <span className="l">{t("Linhas avaliadas", "Rows evaluated")}</span>
          <span className="v">{fmtInt(s.scored)}</span>
          <span className="l">{s.errors ? t(`${s.errors} com erro`, `${s.errors} with errors`) : t("nenhum erro", "no errors")}</span>
        </div>
        <div className="card kpi">
          <span className="l">{t("Nota média", "Mean score")}</span>
          <span className="v">{fmtScore(s.mean_score)}</span>
          <span className="l">{pct(s.pass_rate)} {t("aprovadas", "passed")}</span>
        </div>
        {hasHuman && (
          <div className="card kpi">
            <span className="l">{t("Concorda com você", "Agrees with you")}</span>
            <span className="v">{pct(s.agreement)}</span>
            <span className="l">{t(`em ${fmtInt(s.labeled)} linhas com veredito`, `on ${fmtInt(s.labeled)} rows with a verdict`)}{s.unlabeled ? t(` (${s.unlabeled} sem veredito)`, ` (${s.unlabeled} without a verdict)`) : ""}</span>
          </div>
        )}
        {hasHuman && s.best_threshold != null && (
          <div className="card kpi">
            <span className="l">{t("Melhor limiar", "Best threshold")}</span>
            <span className="v">{fmtScore(s.best_threshold)}</span>
            <span className="l">{t("concordaria em", "would agree on")} {pct(s.best_agreement)}</span>
            {Math.abs(s.best_threshold - props.evaluator.threshold) > 0.001 && (
              <button className="btn sm" style={{ marginTop: 6 }} onClick={useBestThreshold}>{t("Usar este limiar", "Use this threshold")}</button>
            )}
          </div>
        )}
      </div>

      {hasHuman && s.labeled > 0 && (
        <div className="scroll-x">
          <div style={{ minWidth: 520 }}>
            <div className="grid-h" style={{ gridTemplateColumns: "1.2fr 1fr 1fr" }}>
              <span />
              <span>{t("Avaliador aprovou", "Evaluator passed")}</span>
              <span>{t("Avaliador reprovou", "Evaluator failed")}</span>
            </div>
            <div className="grid-r" style={{ gridTemplateColumns: "1.2fr 1fr 1fr" }}>
              <strong>{t("Você aprovou", "You passed")}</strong>
              <span><span className="pill ok">{s.both_pass}</span> {t("concordam", "agree")}</span>
              <span><span className="pill warn">{s.only_human}</span> {t("rígido demais", "too strict")}</span>
            </div>
            <div className="grid-r" style={{ gridTemplateColumns: "1.2fr 1fr 1fr" }}>
              <strong>{t("Você reprovou", "You failed")}</strong>
              <span><span className="pill warn">{s.only_eval}</span> {t("permissivo demais", "too lenient")}</span>
              <span><span className="pill ok">{s.both_fail}</span> {t("concordam", "agree")}</span>
            </div>
          </div>
        </div>
      )}

      <div className="row" style={{ gap: 8 }}>
        {(
          [
            ["all", t("Todas as linhas", "All rows")],
            ...(hasHuman ? [["disagree", t("Só discordâncias", "Only disagreements")]] : []),
            ["errors", t("Só erros", "Only errors")],
          ] as [Filter, string][]
        ).map(([k, label]) => (
          <button key={k} className="chipbtn" aria-pressed={filter === k} onClick={() => setFilter(k)}>{label}</button>
        ))}
      </div>

      <div className="scroll-x">
        <div style={{ minWidth: 900 }}>
          <div className="grid-h" style={{ gridTemplateColumns: rowColumns }}>
            <span>#</span><span>Input</span><span>{t("Resposta", "Answer")}</span>{hasHuman && <span>{t("Você", "You")}</span>}<span>{t("Nota", "Score")}</span><span>{t("Avaliador", "Evaluator")}</span><span>{t("Motivo", "Reason")}</span>
          </div>
          {visible.length === 0 && <p className="muted" style={{ padding: 16 }}>{t("Nenhuma linha neste filtro.", "No rows in this filter.")}</p>}
          {visible.map((r) => {
            return (
              <div key={r.row_index} className="grid-r" style={{ gridTemplateColumns: rowColumns, minHeight: 48 }}>
                <span className="mono muted">{r.row_index + 1}</span>
                <span className="cut" title={r.question}>{r.question || "—"}</span>
                <span className="cut" title={r.answer}>{r.answer}</span>
                {hasHuman && <span className="cut" title={r.human}>{r.human || "—"}</span>}
                <span className="mono">{fmtScore(r.score)}</span>
                <span>
                  {r.error ? (
                    <span className="pill err">{t("Erro", "Error")}</span>
                  ) : (
                    <span className={`pill ${r.passed ? "ok" : "draft"}`}>{r.passed ? t("Aprovou", "Passed") : t("Reprovou", "Failed")}</span>
                  )}
                  {r.agrees === false && <span className="pill warn" style={{ marginLeft: 4 }}>{t("discorda", "disagrees")}</span>}
                </span>
                <span className="cut small muted" title={r.error || r.reason}>{r.error || r.reason}</span>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

const rowColumnsWithHuman = "40px minmax(140px,1.2fr) minmax(160px,1.5fr) 90px 60px 170px minmax(160px,1.5fr)";
const rowColumnsWithoutHuman = "40px minmax(140px,1.2fr) minmax(160px,1.5fr) 60px 110px minmax(160px,1.5fr)";
