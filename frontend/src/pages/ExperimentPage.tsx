// Screens 2–4: one experiment with tabs (Configuração, Resultados, Avaliação, Arquivo).
//
// This component owns the experiment being edited. Changes made in the tabs are
// saved automatically to experiments/<id>.yaml after a short pause, so nobody
// loses work by forgetting to click "Salvar".

import { useEffect, useRef, useState } from "react";
import { Link, NavLink, Route, Routes, useNavigate, useParams } from "react-router-dom";
import { api, type Estimate, type Experiment } from "../api";
import { Dialog } from "../components/Dialog";
import { useToast } from "../components/Toast";
import { fmtCompact, fmtInt, fmtUSD } from "../format";
import { useLoad } from "../hooks";
import ConfigTab from "./ConfigTab";
import EvaluationTab from "./EvaluationTab";
import FileTab from "./FileTab";
import ResultsTab from "./ResultsTab";
import { t } from "../i18n";

export default function ExperimentPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const toast = useToast();

  const loaded = useLoad(() => api.experiment(id), [id]);
  const [exp, setExp] = useState<Experiment | null>(null);
  const [changed, setChanged] = useState(false);
  const [saveState, setSaveState] = useState<"saved" | "saving" | "error">("saved");
  const [showRun, setShowRun] = useState(false);
  const [showDelete, setShowDelete] = useState(false);
  const pendingSave = useRef(false);

  // Copy the loaded experiment into local, editable state.
  useEffect(() => {
    if (loaded.data) {
      setExp(loaded.data.experiment);
      setChanged(loaded.data.changed);
    }
  }, [loaded.data]);

  // Auto-save 700 ms after the last change.
  useEffect(() => {
    if (!exp || !pendingSave.current) return;
    const timer = setTimeout(() => save(exp), 700);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [exp]);

  async function save(value: Experiment) {
    pendingSave.current = false;
    try {
      const result = await api.saveExperiment(value);
      setChanged(result.changed);
      setSaveState("saved");
    } catch (e) {
      setSaveState("error");
      toast(t("Não foi possível salvar: ", "Could not save: ") + (e as Error).message, "err");
    }
  }

  // Tabs call update() for every edit.
  function update(next: Experiment) {
    pendingSave.current = true;
    setSaveState("saving");
    setExp(next);
  }

  async function openRunDialog() {
    if (exp && pendingSave.current) await save(exp); // make sure the run uses the latest edits
    setShowRun(true);
  }

  async function duplicate() {
    if (!exp) return;
    try {
      const copy = await api.createExperiment({
        name: exp.name + t(" (cópia)", " (copy)"),
        description: exp.description,
        use_case: exp.use_case,
        duplicate_from: exp.id,
      });
      toast(t("Experimento duplicado.", "Experiment duplicated."));
      navigate(`/experimentos/${copy.id}`);
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }

  if (loaded.error) {
    return (
      <div className="banner err">
        {loaded.error} <Link to="/" className="btn sm">{t("Voltar", "Back")}</Link>
      </div>
    );
  }
  if (!exp) return <p className="muted">{t("Carregando…", "Loading…")}</p>;

  const base = `/experimentos/${exp.id}`;
  const tabClass = ({ isActive }: { isActive: boolean }) => (isActive ? "active" : "");

  return (
    <>
      <header className="col-flex" style={{ gap: 14 }}>
        <div className="muted small">
          <Link to="/" style={{ textDecoration: "underline" }}>
            {t("Experimentos", "Experiments")}
          </Link>{" "}
          / {exp.name}
        </div>
        <div className="row" style={{ justifyContent: "space-between", gap: 16 }}>
          <div className="row">
            <h1>{exp.name}</h1>
            {changed && <span className="pill warn">{t("Alterações não executadas", "Changes not run yet")}</span>}
            <span className="muted tiny">
              {saveState === "saving" ? t("Salvando…", "Saving…") : saveState === "error" ? t("Erro ao salvar", "Error saving") : t("Salvo", "Saved")}
            </span>
          </div>
          <div className="row" style={{ gap: 10 }}>
            <button className="btn danger" onClick={() => setShowDelete(true)}>
              {t("Excluir", "Delete")}
            </button>
            <button className="btn" onClick={duplicate}>
              {t("Duplicar", "Duplicate")}
            </button>
            <button className="btn pri" onClick={openRunDialog}>
              <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true">
                <path d="M3 2l9 5-9 5z" fill="currentColor" />
              </svg>
              {t("Rodar experimento", "Run experiment")}
            </button>
          </div>
        </div>
        <nav className="tabs" aria-label={t("Seções do experimento", "Experiment sections")}>
          <NavLink to={base} end className={tabClass}>
            {t("Configuração", "Setup")}
          </NavLink>
          <NavLink to={`${base}/resultados`} className={tabClass}>
            {t("Resultados", "Results")}
          </NavLink>
          <NavLink to={`${base}/avaliacao`} className={tabClass}>
            {t("Avaliação", "Evaluation")}
          </NavLink>
          <NavLink to={`${base}/arquivo`} className={tabClass}>
            {t("Arquivo", "File")}
          </NavLink>
        </nav>
      </header>

      <Routes>
        <Route index element={<ConfigTab exp={exp} update={update} onRun={openRunDialog} />} />
        <Route path="resultados" element={<ResultsTab exp={exp} onRun={openRunDialog} />} />
        <Route path="avaliacao" element={<EvaluationTab exp={exp} update={update} />} />
        <Route path="arquivo" element={<FileTab exp={exp} onSaved={loaded.reload} />} />
      </Routes>

      {showRun && (
        <RunDialog
          exp={exp}
          onClose={() => setShowRun(false)}
          onStarted={(runId) => {
            setShowRun(false);
            setChanged(false);
            navigate(`${base}/resultados?execucao=${runId}`);
          }}
        />
      )}
      {showDelete && <DeleteExperimentDialog exp={exp} onClose={() => setShowDelete(false)} onDeleted={() => navigate("/")} />}
    </>
  );
}

// DeleteExperimentDialog says exactly what goes away: the YAML, the runs and
// the experiment's own data folder (data/<id>/). Shared files in data/ stay.
function DeleteExperimentDialog({ exp, onClose, onDeleted }: { exp: Experiment; onClose: () => void; onDeleted: () => void }) {
  const toast = useToast();
  const data = useLoad(() => api.experimentData(exp.id), [exp.id]);
  const usage = data.data?.usage;
  const hasData = !!usage && (usage.files > 0 || usage.images > 0);
  const mb = usage ? usage.bytes / (1024 * 1024) : 0;

  async function remove() {
    try {
      await api.deleteExperiment(exp.id);
      toast(t("Experimento excluído.", "Experiment deleted."));
      onDeleted();
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }

  return (
    <Dialog title={t("Excluir experimento?", "Delete experiment?")} onClose={onClose}>
      <p>
        {t("O arquivo ", "The file ")}
        <span className="mono">experiments/{exp.id}.yaml</span>
        {t(" e todas as execuções deste experimento serão apagados.", " and all runs of this experiment will be deleted.")}
      </p>
      {hasData && usage && (
        <p>
          {t("A pasta ", "The folder ")}
          <span className="mono">data/{exp.id}/</span>
          {t(
            ` também será apagada: ${fmtInt(usage.files)} arquivo(s) de dados e ${fmtInt(usage.images)} imagem(ns), ${mb < 0.1 ? "menos de 0,1" : mb.toFixed(1).replace(".", ",")} MB.`,
            ` will be deleted too: ${fmtInt(usage.files)} data file(s) and ${fmtInt(usage.images)} image(s), ${mb < 0.1 ? "under 0.1" : mb.toFixed(1)} MB.`,
          )}
        </p>
      )}
      <p className="muted">
        {t("Arquivos compartilhados, direto na pasta ", "Shared files, directly in the ")}
        <span className="mono">data/</span>
        {t(", continuam lá.", " folder, stay where they are.")}
      </p>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn" onClick={onClose}>
          {t("Cancelar", "Cancel")}
        </button>
        <button className="btn danger solid" disabled={data.loading} onClick={remove}>
          {t("Excluir", "Delete")}
        </button>
      </div>
    </Dialog>
  );
}

// RunDialog shows what a run will cost BEFORE anything is sent to a paid API.
function RunDialog({ exp, onClose, onStarted }: { exp: Experiment; onClose: () => void; onStarted: (runId: number) => void }) {
  const toast = useToast();
  const [limit, setLimit] = useState(1); // start safe: test with one input first
  const [starting, setStarting] = useState(false);
  const est = useLoad(() => api.estimate(exp.id, limit), [exp.id, limit]);

  async function start() {
    setStarting(true);
    try {
      const { run_id } = await api.startRun(exp.id, limit);
      onStarted(run_id);
    } catch (e) {
      toast((e as Error).message, "err");
      setStarting(false);
    }
  }

  const data = est.data;
  const e: Estimate | undefined = data?.estimate;

  return (
    <Dialog title={t("Rodar experimento", "Run experiment")} onClose={onClose}>
      {est.error && <div className="banner err">{est.error}</div>}
      {data && !data.ready && (
        <div className="banner warn">
          <strong>{t("Falta configurar:", "Still missing:")}</strong> {data.problem}
        </div>
      )}
      {e && (
        <>
          <div role="radiogroup" aria-label={t("Quantidade de inputs", "Number of inputs")} className="col-flex" style={{ gap: 8 }}>
            <label className="tgl">
              <input type="radio" name="limit" checked={limit === 1} onChange={() => setLimit(1)} />
              {t("Testar com 1 input primeiro (recomendado)", "Test with 1 input first (recommended)")}
            </label>
            <label className="tgl">
              <input type="radio" name="limit" checked={limit === 0} onChange={() => setLimit(0)} />
              {t("Rodar todos os inputs", "Run all inputs")}
            </label>
          </div>
          <div>
            <div className="sum">
              <span className="muted">Inputs</span>
              <span className="mono">{fmtInt(e.inputs)}</span>
            </div>
            <div className="sum">
              <span className="muted">{t("Chamadas aos modelos", "Model calls")}</span>
              <span className="mono">{fmtInt(e.calls)}</span>
            </div>
            <div className="sum">
              <span className="muted">{t("Tokens (entrada / saída, aprox.)", "Tokens (input / output, approx.)")}</span>
              <span className="mono">
                ~{fmtCompact(e.input_tokens)} / ~{fmtCompact(e.output_tokens)}
              </span>
            </div>
            <div className="sum">
              <span className="muted">{t("Custo estimado em APIs pagas", "Estimated cost on paid APIs")}</span>
              <strong className="mono">{fmtUSD(e.cost_usd)}</strong>
            </div>
          </div>
          {e.models_without_price.length > 0 && (
            <div className="banner warn small">
              {t("Sem preço configurado para ", "No price configured for ")}
              {e.models_without_price.join(", ")}
              {t(
                ": o custo real pode ser maior. Adicione o preço em Configurações › bancada.yaml (seção pricing).",
                ": the real cost may be higher. Add the price in Settings › bancada.yaml (pricing section).",
              )}
            </div>
          )}
          {e.paid_calls > 0 && (
            <p className="hint">
              {fmtInt(e.paid_calls)}
              {t(
                " chamadas vão para APIs pagas e serão cobradas na sua conta do provedor. A estimativa supõe respostas de ~400 tokens; o valor real aparece nos resultados.",
                " calls go to paid APIs and will be billed to your provider account. The estimate assumes ~400-token answers; the real value shows up in the results.",
              )}
            </p>
          )}
        </>
      )}
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn" onClick={onClose}>
          {t("Cancelar", "Cancel")}
        </button>
        <button className="btn pri" onClick={start} disabled={!data?.ready || starting}>
          {starting
            ? t("Iniciando…", "Starting…")
            : limit === 1
              ? t("Rodar teste", "Run test")
              : t(`Rodar ${fmtInt(e?.calls ?? 0)} chamadas`, `Run ${fmtInt(e?.calls ?? 0)} calls`)}
        </button>
      </div>
    </Dialog>
  );
}
