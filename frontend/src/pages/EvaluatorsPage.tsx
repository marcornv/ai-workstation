// Avaliadores: create, edit and delete evaluators (shared by all experiments).

import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, type Evaluator } from "../api";
import { Dialog } from "../components/Dialog";
import { EvaluatorForm, evaluatorTemplates } from "../components/EvaluatorForm";
import { EvaluatorSheetTest } from "../components/EvaluatorSheetTest";
import { useToast } from "../components/Toast";
import { evaluatorKindLabel } from "../format";
import { useLoad } from "../hooks";
import { t } from "../i18n";

type Draft = Omit<Evaluator, "id"> & { id?: string };

export default function EvaluatorsPage() {
  const toast = useToast();
  const list = useLoad(() => api.evaluators(), []);
  const [editing, setEditing] = useState<Draft | null>(null);
  // "/avaliadores?avaliador=<id>" opens that evaluator directly.
  const [params] = useSearchParams();
  const wanted = params.get("avaliador");
  useEffect(() => {
    const match = list.data?.find((e) => e.id === wanted);
    if (match) setEditing(match);
  }, [list.data, wanted]);
  const [deleting, setDeleting] = useState<Evaluator | null>(null);

  async function save(ev: Draft) {
    try {
      const saved = ev.id ? await api.saveEvaluator(ev as Evaluator) : await api.createEvaluator(ev);
      toast(t("Avaliador salvo.", "Evaluator saved."));
      setEditing(saved);
      list.reload();
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }

  return (
    <>
      <header className="col-flex" style={{ gap: 6, maxWidth: 700 }}>
        <h1 style={{ fontSize: 32 }}>{t("Avaliadores", "Evaluators")}</h1>
        <p className="muted">
          {t(
            "Avaliadores dão uma nota de 0 a 1 para cada resposta. Crie aqui e ative em cada experimento, na aba Avaliação. Cada um vive em ",
            "Evaluators give each answer a score from 0 to 1. Create them here and enable them in each experiment, in the Evaluation tab. Each one lives in ",
          )}
          <span className="mono">evaluators/&lt;{t("nome", "name")}&gt;.yaml</span>.
        </p>
      </header>

      <div className="row" style={{ alignItems: "flex-start", gap: 24 }}>
        <section className="card sec" style={{ flex: "0 1 400px", minWidth: 0, width: "100%" }}>
          <h2>{t("Seus avaliadores", "Your evaluators")}</h2>
          {list.error && <div className="banner err">{list.error}</div>}
          {list.data?.length === 0 && <p className="muted">{t("Nenhum ainda. Comece por um modelo abaixo.", "None yet. Start from a template below.")}</p>}
          <div className="col-flex" style={{ gap: 8 }}>
            {(list.data ?? []).map((ev) => (
              <div key={ev.id} className={`ev ${editing?.id === ev.id ? "sel" : ""}`}>
                <button className="evbtn pad" aria-pressed={editing?.id === ev.id} onClick={() => setEditing(ev)}>
                  <span style={{ fontWeight: 500 }}>{ev.name}</span>
                  <span className="pill draft">{evaluatorKindLabel[ev.kind] ?? ev.kind}</span>
                </button>
                <button className="btn sm danger" style={{ margin: 10 }} aria-label={t(`Excluir ${ev.name}`, `Delete ${ev.name}`)} onClick={() => setDeleting(ev)}>
                  {t("Excluir", "Delete")}
                </button>
              </div>
            ))}
          </div>
          <div className="col-flex" style={{ gap: 10, paddingTop: 6, borderTop: "1px solid var(--line2)" }}>
            <span style={{ fontWeight: 500, fontSize: 13 }}>{t("Criar avaliador", "Create evaluator")}</span>
            <div className="row" style={{ gap: 8 }}>
              <button className="btn sm" onClick={() => setEditing({ ...evaluatorTemplates.ragas })}>{t("Métrica Ragas", "Ragas metric")}</button>
              <button className="btn sm" onClick={() => setEditing({ ...evaluatorTemplates.llm_judge })}>{t("Juiz LLM com rubrica", "LLM judge with rubric")}</button>
              <button className="btn sm" onClick={() => setEditing({ ...evaluatorTemplates.embedding })}>{t("Similaridade (embedding)", "Similarity (embedding)")}</button>
              <button className="btn sm" onClick={() => setEditing({ ...evaluatorTemplates.rule })}>{t("Regra (texto / regex)", "Rule (text / regex)")}</button>
              <button className="btn sm" onClick={() => setEditing({ ...evaluatorTemplates.decision })}>{t("Motor de decisão", "Decision engine")}</button>
            </div>
          </div>
        </section>

        <section className="card sec" style={{ flex: "1 1 460px", minWidth: 0 }}>
          {editing ? (
            <>
              <h2>{editing.id ? editing.name : t("Novo avaliador", "New evaluator")}</h2>
              <EvaluatorForm value={editing} onSave={save} onCancel={() => setEditing(null)} />
            </>
          ) : (
            <p className="muted">{t("Escolha um avaliador à esquerda ou crie um novo.", "Pick an evaluator on the left or create a new one.")}</p>
          )}
        </section>
      </div>

      {/* Only saved evaluators can be tested on a spreadsheet. */}
      {editing?.id && (
        <EvaluatorSheetTest
          key={editing.id}
          evaluator={editing as Evaluator}
          onEvaluatorSaved={(ev) => {
            setEditing(ev);
            list.reload();
          }}
        />
      )}

      {deleting && (
        <Dialog title={t("Excluir avaliador?", "Delete evaluator?")} onClose={() => setDeleting(null)}>
          <p>
            “{deleting.name}”
            {t(
              " será removido de todos os experimentos. Notas já calculadas continuam nas execuções antigas.",
              " will be removed from all experiments. Scores already computed stay in the old runs.",
            )}
          </p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <button className="btn" onClick={() => setDeleting(null)}>{t("Cancelar", "Cancel")}</button>
            <button
              className="btn danger solid"
              onClick={async () => {
                await api.deleteEvaluator(deleting.id);
                if (editing?.id === deleting.id) setEditing(null);
                setDeleting(null);
                list.reload();
                toast(t("Avaliador excluído.", "Evaluator deleted."));
              }}
            >
              {t("Excluir", "Delete")}
            </button>
          </div>
        </Dialog>
      )}
    </>
  );
}
