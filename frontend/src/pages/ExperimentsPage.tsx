// Screen 1: list of experiments + "Novo experimento" dialog.

import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, type ExperimentListItem } from "../api";
import { Dialog } from "../components/Dialog";
import { useToast } from "../components/Toast";
import { fmtAgo, fmtInt, fmtScore, runStatus, useCaseLabel } from "../format";
import { useInterval, useLoad } from "../hooks";
import { t } from "../i18n";

type Filter = "all" | "running" | "done" | "draft";

const filters = (): [Filter, string][] => [
  ["all", t("Todos", "All")],
  ["running", t("Rodando", "Running")],
  ["done", t("Concluídos", "Done")],
  ["draft", t("Rascunhos", "Drafts")],
];

function matchesFilter(e: ExperimentListItem, f: Filter) {
  if (f === "all") return true;
  if (f === "draft") return !e.last_run;
  if (f === "running") return e.last_run?.status === "running";
  return !!e.last_run && e.last_run.status !== "running";
}

const columns = "minmax(260px,2.4fr) 170px minmax(80px,.6fr) 90px 130px 110px";

export default function ExperimentsPage() {
  const { data, error, reload } = useLoad(() => api.experiments(), []);
  const [filter, setFilter] = useState<Filter>("all");
  const [search, setSearch] = useState("");
  const [showNew, setShowNew] = useState(false);

  const anyRunning = !!data?.some((e) => e.last_run?.status === "running");
  useInterval(reload, 2000, anyRunning);

  const term = search.trim().toLowerCase();
  const rows = (data ?? []).filter(
    (e) => matchesFilter(e, filter) && (!term || (e.name + " " + e.description).toLowerCase().includes(term)),
  );

  return (
    <>
      <header className="row" style={{ justifyContent: "space-between", alignItems: "flex-end", gap: 16 }}>
        <div className="col-flex" style={{ gap: 6, maxWidth: 620 }}>
          <h1 style={{ fontSize: 32 }}>{t("Experimentos", "Experiments")}</h1>
          <p className="muted">
            {t(
              "Rode os mesmos dados em vários modelos, compare custo e latência e avalie a qualidade das respostas.",
              "Run the same data on several models, compare cost and latency, and evaluate the quality of the answers.",
            )}
          </p>
        </div>
        <button className="btn pri" onClick={() => setShowNew(true)}>
          <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
            <path d="M8 3v10M3 8h10" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
          </svg>
          {t("Novo experimento", "New experiment")}
        </button>
      </header>

      <div className="row">
        <div style={{ flex: "1 1 280px", maxWidth: 380 }}>
          <label htmlFor="busca" className="sr-only">
            {t("Buscar experimento", "Search experiments")}
          </label>
          <input
            id="busca"
            className="inp"
            type="search"
            placeholder={t("Buscar por nome ou descrição", "Search by name or description")}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <div className="row" style={{ gap: 8 }}>
          {filters().map(([key, label]) => (
            <button key={key} className="chipbtn" aria-pressed={filter === key} onClick={() => setFilter(key)}>
              {label}
            </button>
          ))}
        </div>
      </div>

      {error && <div className="banner err">{error}</div>}

      <section className="card scroll-x">
        {data && data.length === 0 ? (
          <div className="empty">
            <h2>{t("Nenhum experimento ainda", "No experiments yet")}</h2>
            <p className="muted">{t("Crie o primeiro para comparar modelos com os seus próprios dados.", "Create the first one to compare models on your own data.")}</p>
            <button className="btn pri" onClick={() => setShowNew(true)}>
              {t("Novo experimento", "New experiment")}
            </button>
          </div>
        ) : (
          <div style={{ minWidth: 960 }}>
            <div className="grid-h" style={{ gridTemplateColumns: columns }}>
              <span>{t("Experimento", "Experiment")}</span>
              <span>Status</span>
              <span>{t("Modelos", "Models")}</span>
              <span>Inputs</span>
              <span>{t("Última execução", "Last run")}</span>
              <span>{t("Nota média", "Mean score")}</span>
            </div>
            {rows.map((e) => {
              const status = runStatus(e.last_run);
              const running = e.last_run?.status === "running";
              const progress = e.last_run && e.last_run.total > 0 ? (e.last_run.done / e.last_run.total) * 100 : 0;
              return (
                <Link
                  key={e.id}
                  to={`/experimentos/${e.id}`}
                  className="grid-r"
                  style={{ gridTemplateColumns: columns, minHeight: 76 }}
                >
                  <span className="col-flex" style={{ gap: 2 }}>
                    <strong style={{ fontWeight: 600 }}>{e.name}</strong>
                    <span className="muted small">{e.description || useCaseLabel[e.use_case] || ""}</span>
                  </span>
                  <span>
                    <span className={`pill ${status.cls}`}>{status.label}</span>
                    {e.changed && !running && (
                      <span className="muted tiny" style={{ display: "block", marginTop: 4 }}>
                        {t("alterado desde a execução", "changed since the run")}
                      </span>
                    )}
                    {running && (
                      <span className="bar" aria-hidden="true" style={{ display: "block" }}>
                        <i style={{ width: `${progress}%` }} />
                      </span>
                    )}
                  </span>
                  <span className="mono">{e.enabled_models}</span>
                  <span className="mono">{fmtInt(e.inputs)}</span>
                  <span className="muted">{fmtAgo(e.last_run?.started_at)}</span>
                  <span className="mono">{fmtScore(e.mean_score)}</span>
                </Link>
              );
            })}
          </div>
        )}
      </section>
      <p className="muted small">
        {t("Cada experimento é um arquivo em ", "Each experiment is a file in ")}
        <span className="mono">experiments/</span>.{" "}
        {t("Você pode editar aqui ou direto no arquivo.", "You can edit it here or directly in the file.")}
      </p>

      {showNew && <NewExperimentDialog experiments={data ?? []} onClose={() => setShowNew(false)} />}
    </>
  );
}

function NewExperimentDialog({ experiments, onClose }: { experiments: ExperimentListItem[]; onClose: () => void }) {
  const navigate = useNavigate();
  const toast = useToast();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [useCase, setUseCase] = useState("free");
  const [base, setBase] = useState("");
  const [saving, setSaving] = useState(false);

  async function create() {
    if (!name.trim()) {
      toast(t("Dê um nome ao experimento.", "Give the experiment a name."), "err");
      return;
    }
    setSaving(true);
    try {
      const exp = await api.createExperiment({ name, description, use_case: useCase, duplicate_from: base || undefined });
      navigate(`/experimentos/${exp.id}`);
    } catch (e) {
      toast((e as Error).message, "err");
      setSaving(false);
    }
  }

  return (
    <Dialog title={t("Novo experimento", "New experiment")} onClose={onClose}>
      <div className="field">
        <label htmlFor="n-nome">{t("Nome", "Name")}</label>
        <input
          id="n-nome"
          className="inp"
          autoFocus
          placeholder={t("Ex.: Resumo de contratos", "E.g.: Contract summaries")}
          value={name}
          onChange={(e) => setName(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && create()}
        />
      </div>
      <div className="field">
        <label htmlFor="n-desc">{t("Descrição (opcional)", "Description (optional)")}</label>
        <input
          id="n-desc"
          className="inp"
          placeholder={t("O que você quer descobrir com este experimento?", "What do you want to find out with this experiment?")}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </div>
      <div className="field">
        <label htmlFor="n-tipo">{t("Tipo de caso de uso", "Use case")}</label>
        <select id="n-tipo" className="inp" value={useCase} onChange={(e) => setUseCase(e.target.value)}>
          {Object.entries(useCaseLabel).map(([k, label]) => (
            <option key={k} value={k}>
              {label}
            </option>
          ))}
        </select>
        <span className="hint">{t("Ajuda a lembrar o objetivo e a escolher os avaliadores.", "Helps you remember the goal and pick evaluators.")}</span>
      </div>
      <div className="field">
        <label htmlFor="n-base">{t("Partir de", "Start from")}</label>
        <select id="n-base" className="inp" value={base} onChange={(e) => setBase(e.target.value)}>
          <option value="">{t("Experimento em branco", "Blank experiment")}</option>
          {experiments.map((e) => (
            <option key={e.id} value={e.id}>
              {t("Duplicar: ", "Duplicate: ")}{e.name}
            </option>
          ))}
        </select>
      </div>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn" onClick={onClose}>
          {t("Cancelar", "Cancel")}
        </button>
        <button className="btn pri" onClick={create} disabled={saving}>
          {saving ? t("Criando…", "Creating…") : t("Criar e configurar", "Create and configure")}
        </button>
      </div>
    </Dialog>
  );
}
