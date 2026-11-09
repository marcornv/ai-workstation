// Experiment › Configuração: models, prompt, input data and run settings.

import { useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api, datasetOwner, imageURL, type Experiment, type ModelConfig, type ProviderView } from "../api";
import { Dialog } from "../components/Dialog";
import { useToast } from "../components/Toast";
import { fmtCompact, fmtInt, fmtUSD, modelPlaceholder, providerKindLabel, providerTag } from "../format";
import { useLoad } from "../hooks";
import { t } from "../i18n";

type Props = { exp: Experiment; update: (e: Experiment) => void; onRun: () => void };

export default function ConfigTab({ exp, update, onRun }: Props) {
  return (
    <div className="row" style={{ alignItems: "flex-start", gap: 24 }}>
      <div className="col-flex" style={{ flex: "1 1 640px", minWidth: 0, gap: 24 }}>
        <ModelsSection exp={exp} update={update} />
        <PromptSection exp={exp} update={update} />
        <InputsSection exp={exp} update={update} />
      </div>
      <aside className="col-flex" style={{ flex: "0 0 320px", maxWidth: "100%", gap: 24 }}>
        <RunSettingsSection exp={exp} update={update} />
        <EstimateSection exp={exp} onRun={onRun} />
      </aside>
    </div>
  );
}

// ---------- Models ----------

function ModelsSection({ exp, update }: { exp: Experiment; update: (e: Experiment) => void }) {
  const settings = useLoad(() => api.settings(), []);
  const providers = settings.data?.providers ?? [];
  const [adding, setAdding] = useState<ProviderView | null>(null);
  const active = exp.models.filter((m) => m.enabled).length;

  function setModel(index: number, patch: Partial<ModelConfig>) {
    const models = exp.models.map((m, i) => (i === index ? { ...m, ...patch } : m));
    update({ ...exp, models });
  }
  function removeModel(index: number) {
    update({ ...exp, models: exp.models.filter((_, i) => i !== index) });
  }

  return (
    <section className="card sec">
      <div className="sechead">
        <h2>{t("Modelos", "Models")}</h2>
        <span className="muted">
          {t(`${active} de ${exp.models.length} ativos`, `${active} of ${exp.models.length} enabled`)}
        </span>
      </div>

      {exp.models.length === 0 && <p className="muted">{t("Nenhum modelo ainda. Adicione pelo menos um abaixo.", "No models yet. Add at least one below.")}</p>}

      <div className="col-flex" style={{ gap: 10 }}>
        {exp.models.map((m, i) => {
          const provider = providers.find((p) => p.name === m.provider);
          return (
            <div key={i} className={`model ${m.enabled ? "" : "off"}`}>
              <div className="row" style={{ gap: 12, minWidth: 0, flexWrap: "nowrap" }}>
                <span className="prov" aria-hidden="true">
                  {provider ? providerTag[provider.kind] : "?"}
                </span>
                <div style={{ minWidth: 0 }}>
                  <div className="mono mname cut" style={{ fontWeight: 500 }}>
                    {m.model}
                  </div>
                  <div className="muted small">{m.provider}</div>
                </div>
              </div>
              <KeyStatus provider={provider} />
              <div className="field">
                <label className="tiny muted" htmlFor={`temp-${i}`}>
                  {t("Temperatura", "Temperature")}
                </label>
                <input
                  id={`temp-${i}`}
                  className="inp mono"
                  type="number"
                  min={0}
                  max={2}
                  step={0.1}
                  placeholder={t("padrão", "default")}
                  disabled={provider?.kind === "anthropic"}
                  title={provider?.kind === "anthropic" ? t("Os modelos Claude atuais não aceitam temperatura", "Current Claude models do not accept temperature") : ""}
                  value={m.temperature ?? ""}
                  onChange={(e) => setModel(i, { temperature: e.target.value === "" ? null : Number(e.target.value) })}
                />
              </div>
              <div className="field">
                <label className="tiny muted" htmlFor={`max-${i}`}>
                  {t("Máx. tokens", "Max tokens")}
                </label>
                <input
                  id={`max-${i}`}
                  className="inp mono"
                  type="number"
                  min={1}
                  placeholder="4096"
                  value={m.max_tokens || ""}
                  onChange={(e) => setModel(i, { max_tokens: Number(e.target.value) || 0 })}
                />
              </div>
              <div className="row" style={{ gap: 8, justifyContent: "flex-end", flexWrap: "nowrap" }}>
                <label className="tgl">
                  <input type="checkbox" checked={m.enabled} onChange={() => setModel(i, { enabled: !m.enabled })} />{" "}
                  {t("Ativo", "Enabled")}
                </label>
                <button className="btn sm danger" aria-label={t(`Remover ${m.model}`, `Remove ${m.model}`)} onClick={() => removeModel(i)}>
                  {t("Remover", "Remove")}
                </button>
              </div>
            </div>
          );
        })}
      </div>

      <div className="col-flex" style={{ gap: 10, paddingTop: 6, borderTop: "1px solid var(--line2)" }}>
        <span style={{ fontWeight: 500, fontSize: 13 }}>{t("Adicionar modelo", "Add model")}</span>
        <div className="row" style={{ gap: 8 }}>
          {providers.filter((p) => p.kind !== "typesafe").map((p) => (
            <button key={p.name} className="btn sm" onClick={() => setAdding(p)}>
              {p.name === p.kind ? providerKindLabel[p.kind] : `${p.name} (${providerKindLabel[p.kind]})`}
            </button>
          ))}
          <Link to="/configuracoes" className="btn sm link">
            {t("Gerenciar provedores", "Manage providers")}
          </Link>
        </div>
      </div>

      {adding && (
        <AddModelDialog
          provider={adding}
          onClose={() => setAdding(null)}
          onAdd={(model) => {
            update({
              ...exp,
              models: [...exp.models, { model, provider: adding.name, enabled: true, temperature: null, max_tokens: 0 }],
            });
            setAdding(null);
          }}
        />
      )}
    </section>
  );
}

function KeyStatus({ provider }: { provider?: ProviderView }) {
  if (!provider) return <span className="pill err">{t("Provedor não existe", "Provider does not exist")}</span>;
  if (!provider.api_key_env) return <span className="pill run">{t("Sem chave (local)", "No key (local)")}</span>;
  return (
    <div>
      {provider.key_set ? (
        <span className="pill ok">{t("Chave configurada", "Key configured")}</span>
      ) : (
        <Link to="/configuracoes" className="pill warn">
          {t("Falta a chave", "Key missing")}
        </Link>
      )}
      <div className="mono muted tiny" style={{ marginTop: 4 }}>
        {provider.api_key_env}
      </div>
    </div>
  );
}

function AddModelDialog({ provider, onClose, onAdd }: { provider: ProviderView; onClose: () => void; onAdd: (m: string) => void }) {
  const [model, setModel] = useState("");
  // Ask the provider which models exist, to offer suggestions (works offline-safe: errors are just shown).
  const available = useLoad(() => api.providerModels(provider.name), [provider.name]);

  return (
    <Dialog title={t(`Adicionar modelo · ${providerKindLabel[provider.kind]}`, `Add model · ${providerKindLabel[provider.kind]}`)} onClose={onClose}>
      <div className="field">
        <label htmlFor="add-model">{t("Nome do modelo", "Model name")}</label>
        <input
          id="add-model"
          className="inp mono"
          autoFocus
          list="model-suggestions"
          placeholder={modelPlaceholder[provider.kind]}
          value={model}
          onChange={(e) => setModel(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && model.trim() && onAdd(model.trim())}
        />
        <datalist id="model-suggestions">
          {(available.data?.models ?? []).map((m) => (
            <option key={m} value={m} />
          ))}
        </datalist>
        {available.loading && <span className="hint">{t("Buscando modelos disponíveis…", "Looking for available models…")}</span>}
        {available.data && <span className="hint">{(available.data.models ?? []).length}{t(" modelos encontrados. Comece a digitar para ver sugestões.", " models found. Start typing to see suggestions.")}</span>}
        {available.error && <span className="hint">{t("Não foi possível listar os modelos", "Could not list the models")} ({available.error}). {t("Você ainda pode digitar o nome.", "You can still type the name.")}</span>}
      </div>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn" onClick={onClose}>
          {t("Cancelar", "Cancel")}
        </button>
        <button className="btn pri" disabled={!model.trim()} onClick={() => onAdd(model.trim())}>
          {t("Adicionar", "Add")}
        </button>
      </div>
    </Dialog>
  );
}

// ---------- Prompt ----------

function PromptSection({ exp, update }: { exp: Experiment; update: (e: Experiment) => void }) {
  const preview = useLoad(
    () => (exp.dataset.file ? api.previewDataset(exp.dataset.file, 1) : Promise.resolve(null)),
    [exp.dataset.file],
  );
  const userRef = useRef<HTMLTextAreaElement>(null);
  const columns = preview.data?.columns ?? [];

  // Insert "{column}" where the cursor is in the user message.
  function insertColumn(col: string) {
    const el = userRef.current;
    const text = exp.prompt.user;
    const at = el ? el.selectionStart : text.length;
    const next = text.slice(0, at) + `{${col}}` + text.slice(el ? el.selectionEnd : at);
    update({ ...exp, prompt: { ...exp.prompt, user: next } });
  }

  return (
    <section className="card sec">
      <div className="sechead">
        <h2>Prompt</h2>
        <span className="muted">
          {t("Use ", "Use ")}<span className="mono">{t("{coluna}", "{column}")}</span>{t(" para inserir valores dos dados", " to insert values from the data")}
        </span>
      </div>
      <div className="field">
        <label htmlFor="p-sys">{t("Instrução do sistema (opcional)", "System instruction (optional)")}</label>
        <textarea
          id="p-sys"
          className="inp"
          rows={3}
          placeholder={t("Ex.: Você é um assistente jurídico. Responda em português.", "E.g.: You are a legal assistant. Answer in English.")}
          value={exp.prompt.system}
          onChange={(e) => update({ ...exp, prompt: { ...exp.prompt, system: e.target.value } })}
        />
      </div>
      <div className="field">
        <label htmlFor="p-usr">{t("Mensagem do usuário", "User message")}</label>
        <textarea
          id="p-usr"
          ref={userRef}
          className="inp"
          rows={4}
          value={exp.prompt.user}
          onChange={(e) => update({ ...exp, prompt: { ...exp.prompt, user: e.target.value } })}
        />
      </div>
      {columns.length > 0 && (
        <div className="row" style={{ gap: 8 }}>
          <span className="hint">{t("Inserir coluna:", "Insert column:")}</span>
          {columns.map((c) => (
            <button key={c} className="btn sm mono" onClick={() => insertColumn(c)}>
              {`{${c}}`}
            </button>
          ))}
        </div>
      )}
    </section>
  );
}

// ---------- Inputs (dataset) ----------

function InputsSection({ exp, update }: { exp: Experiment; update: (e: Experiment) => void }) {
  const toast = useToast();
  const fileInput = useRef<HTMLInputElement>(null);
  const imageInput = useRef<HTMLInputElement>(null);
  const datasets = useLoad(() => api.datasets(exp.id), [exp.id]);
  const preview = useLoad(
    () => (exp.dataset.file ? api.previewDataset(exp.dataset.file, 5) : Promise.resolve(null)),
    [exp.dataset.file],
  );
  const [pasting, setPasting] = useState(false);
  // Image columns of the dataset being labeled; null = dialog closed.
  const [labeling, setLabeling] = useState<string[] | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Switching files resets the column mapping. Columns that look like images
  // (e.g. "images/a.png") are pre-selected as image columns.
  async function chooseFile(file: string) {
    let imageColumns: string[] = [];
    if (file) {
      imageColumns = await api.previewDataset(file, 1).then((p) => p.image_columns_guess, () => []);
    }
    update({ ...exp, dataset: { file, reference_column: "", context_column: "", image_columns: imageColumns } });
    datasets.reload();
  }

  async function uploadImages(files: FileList) {
    try {
      const res = await api.uploadImages("imagens", files, exp.id);
      toast(t(`${res.count} imagem(ns) salvas. Dados em data/${res.file}`, `${res.count} image(s) saved. Data in data/${res.file}`));
      await chooseFile(res.file);
      setLabeling(res.image_columns);
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }

  async function deleteFile() {
    try {
      await api.deleteDataset(exp.dataset.file);
      toast(t(`Arquivo data/${exp.dataset.file} excluído.`, `File data/${exp.dataset.file} deleted.`));
      setDeleting(false);
      chooseFile("");
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }

  function toggleImageColumn(col: string) {
    const current = exp.dataset.image_columns;
    const next = current.includes(col) ? current.filter((c) => c !== col) : [...current, col];
    update({ ...exp, dataset: { ...exp.dataset, image_columns: next } });
  }

  async function upload(file: File) {
    try {
      const res = await api.uploadDataset(file, exp.id);
      toast(t(`Arquivo salvo como data/${res.file}`, `File saved as data/${res.file}`));
      chooseFile(res.file);
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }

  const p = preview.data;
  const columns = p?.columns ?? [];
  // This experiment's own files (data/<id>/) and the shared ones (data/).
  const allFiles = datasets.data ?? [];
  const ownFiles = allFiles.filter((f) => datasetOwner(f) === exp.id);
  const sharedFiles = allFiles.filter((f) => datasetOwner(f) === "");
  const ownsFile = datasetOwner(exp.dataset.file) === exp.id;
  const listed = allFiles.includes(exp.dataset.file);
  // Labels are saved into the file itself, which only works for JSONL.
  const canLabel = /\.(jsonl|ndjson)$/i.test(exp.dataset.file) && exp.dataset.image_columns.length > 0;

  return (
    <section className="card sec">
      <div className="sechead">
        <h2>Inputs</h2>
        <div className="row" style={{ gap: 8 }}>
          <button className="btn sm" onClick={() => setPasting(true)}>
            {t("Colar texto", "Paste text")}
          </button>
          <button className="btn sm" onClick={() => fileInput.current?.click()}>
            {t("Enviar CSV / JSONL", "Upload CSV / JSONL")}
          </button>
          <button className="btn sm" onClick={() => imageInput.current?.click()}>
            {t("Enviar imagens", "Upload images")}
          </button>
          <input
            ref={imageInput}
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp"
            multiple
            hidden
            onChange={(e) => {
              if (e.target.files?.length) uploadImages(e.target.files);
              e.target.value = "";
            }}
          />
          <input
            ref={fileInput}
            type="file"
            accept=".csv,.jsonl,.ndjson"
            hidden
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) upload(f);
              e.target.value = "";
            }}
          />
        </div>
      </div>

      <div className="field">
        <label htmlFor="ds-file">{t("Arquivo de dados", "Data file")}</label>
        <div className="row" style={{ gap: 8, flexWrap: "nowrap" }}>
          <select id="ds-file" className="inp" style={{ flex: "1 1 auto", minWidth: 0 }} value={exp.dataset.file} onChange={(e) => chooseFile(e.target.value)}>
            <option value="">{t("— escolha um arquivo —", "— choose a file —")}</option>
            {exp.dataset.file && !listed && <option value={exp.dataset.file}>{exp.dataset.file}</option>}
            {ownFiles.length > 0 && (
              <optgroup label={t("Deste experimento", "This experiment")}>
                {ownFiles.map((f) => (
                  <option key={f} value={f}>
                    {f.slice(exp.id.length + 1)}
                  </option>
                ))}
              </optgroup>
            )}
            {sharedFiles.length > 0 && (
              <optgroup label={t("Compartilhados (pasta data/)", "Shared (data/ folder)")}>
                {sharedFiles.map((f) => (
                  <option key={f} value={f}>
                    {f}
                  </option>
                ))}
              </optgroup>
            )}
          </select>
          {ownsFile && (
            <button className="btn sm danger" onClick={() => setDeleting(true)} aria-label={t(`Excluir ${exp.dataset.file}`, `Delete ${exp.dataset.file}`)}>
              {t("Excluir", "Delete")}
            </button>
          )}
        </div>
        <span className="hint">
          {t(
            `CSV com cabeçalho ou JSONL (um objeto JSON por linha). Cada linha vira um input; cada coluna pode ser usada no prompt. Arquivos enviados aqui ficam em data/${exp.id}/ e são apagados junto com o experimento.`,
            `CSV with a header or JSONL (one JSON object per line). Each line becomes an input; each column can be used in the prompt. Files uploaded here go to data/${exp.id}/ and are deleted along with the experiment.`,
          )}
        </span>
      </div>

      {preview.error && <div className="banner err">{preview.error}</div>}

      {p && (
        <>
          <div className="card soft row" style={{ padding: "12px 16px", gap: 20 }}>
            <span className="mono" style={{ fontWeight: 500 }}>
              {p.file}
            </span>
            <span className="muted">
              {fmtInt(p.count)} {t("linhas", "rows")} · {columns.length} {t("colunas", "columns")}
            </span>
            {canLabel && (
              <button className="btn sm" style={{ marginLeft: "auto" }} onClick={() => setLabeling(exp.dataset.image_columns)}>
                {t("Rotular imagens", "Label images")}
              </button>
            )}
          </div>

          <div className="row" style={{ alignItems: "flex-start" }}>
            <div className="field" style={{ flex: "1 1 220px" }}>
              <label htmlFor="ds-ref">{t("Resposta esperada (opcional)", "Expected answer (optional)")}</label>
              <select
                id="ds-ref"
                className="inp"
                value={exp.dataset.reference_column}
                onChange={(e) => update({ ...exp, dataset: { ...exp.dataset, reference_column: e.target.value } })}
              >
                <option value="">{t("— nenhuma —", "— none —")}</option>
                {columns.map((c) => (
                  <option key={c}>{c}</option>
                ))}
              </select>
              <span className="hint">{t("Usada por avaliadores que comparam com uma referência.", "Used by evaluators that compare with a reference.")}</span>
            </div>
            <div className="field" style={{ flex: "1 1 220px" }}>
              <label htmlFor="ds-ctx">{t("Contexto recuperado (opcional, RAG)", "Retrieved context (optional, RAG)")}</label>
              <select
                id="ds-ctx"
                className="inp"
                value={exp.dataset.context_column}
                onChange={(e) => update({ ...exp, dataset: { ...exp.dataset, context_column: e.target.value } })}
              >
                <option value="">{t("— nenhuma —", "— none —")}</option>
                {columns.map((c) => (
                  <option key={c}>{c}</option>
                ))}
              </select>
              <span className="hint">{t("Trechos separados por linha em branco ou “---”.", "Chunks separated by a blank line or “---”.")}</span>
            </div>
          </div>

          <div className="field">
            <span className="label">{t("Colunas com imagens", "Image columns")}</span>
            <div className="row" style={{ gap: 4 }}>
              {columns.map((c) => (
                <label key={c} className="tgl" style={{ marginRight: 12 }}>
                  <input type="checkbox" checked={exp.dataset.image_columns.includes(c)} onChange={() => toggleImageColumn(c)} />
                  <span className="mono">{c}</span>
                </label>
              ))}
            </div>
            <span className="hint">
              {t(
                "As imagens dessas colunas são anexadas à mensagem enviada ao modelo. Cada célula deve ter um caminho dentro da pasta data/ (ex.: images/fotos/001.jpg) ou um link https. Use modelos com visão (ex.: GPT-4o, Claude, Gemini, ou no Ollama: gemma3, llava, qwen2.5vl).",
                "Images in these columns are attached to the message sent to the model. Each cell must hold a path inside the data/ folder (e.g. images/photos/001.jpg) or an https link. Use vision models (e.g. GPT-4o, Claude, Gemini, or on Ollama: gemma3, llava, qwen2.5vl).",
              )}
            </span>
          </div>

          <div className="scroll-x">
            <div style={{ minWidth: 160 * columns.length + 40 }}>
              <div className="grid-h" style={{ gridTemplateColumns: `40px repeat(${columns.length}, minmax(140px, 1fr))` }}>
                <span>#</span>
                {columns.map((c) => (
                  <span key={c} className="cut" style={{ textTransform: "none" }}>
                    {c}
                  </span>
                ))}
              </div>
              {p.rows.map((row, i) => (
                <div
                  key={i}
                  className="grid-r"
                  style={{ gridTemplateColumns: `40px repeat(${columns.length}, minmax(140px, 1fr))`, minHeight: 48, padding: "6px 16px" }}
                >
                  <span className="mono muted">{i + 1}</span>
                  {columns.map((c) =>
                    exp.dataset.image_columns.includes(c) && row[c] ? (
                      <img key={c} src={imageURL(row[c])} alt={row[c]} title={row[c]} className="thumb" />
                    ) : (
                      <span key={c} className="cut" title={row[c]}>
                        {row[c]}
                      </span>
                    ),
                  )}
                </div>
              ))}
            </div>
          </div>
          {p.count > p.rows.length && <span className="hint">{t(`Mostrando as primeiras ${p.rows.length} linhas.`, `Showing the first ${p.rows.length} rows.`)}</span>}
        </>
      )}

      {pasting && <PasteDialog experimentId={exp.id} onClose={() => setPasting(false)} onSaved={(f) => { setPasting(false); chooseFile(f); }} />}
      {deleting && (
        <Dialog title={t("Excluir arquivo?", "Delete file?")} onClose={() => setDeleting(false)}>
          <p>
            {t("O arquivo ", "The file ")}
            <span className="mono">data/{exp.dataset.file}</span>
            {/\.(jsonl|ndjson)$/i.test(exp.dataset.file) && exp.dataset.image_columns.length > 0
              ? t(" e as imagens enviadas com ele serão apagados.", " and the images uploaded with it will be deleted.")
              : t(" será apagado.", " will be deleted.")}{" "}
            {t(
              "Os resultados de execuções anteriores continuam salvos, mas para rodar de novo será preciso escolher outro arquivo.",
              "Results of earlier runs stay saved, but you'll need to choose another file to run again.",
            )}
          </p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <button className="btn" onClick={() => setDeleting(false)}>
              {t("Cancelar", "Cancel")}
            </button>
            <button className="btn danger solid" onClick={deleteFile}>
              {t("Excluir", "Delete")}
            </button>
          </div>
        </Dialog>
      )}
      {labeling && exp.dataset.file && (
        <LabelDialog
          file={exp.dataset.file}
          imageColumns={labeling}
          initialColumn={exp.dataset.reference_column}
          onClose={() => setLabeling(null)}
          onSaved={(column) => {
            setLabeling(null);
            // The labels become the expected answer that evaluators compare against.
            update({ ...exp, dataset: { ...exp.dataset, reference_column: column } });
            preview.reload();
          }}
        />
      )}
    </section>
  );
}

// LabelDialog shows every image of a dataset with a text box for its label,
// and saves the labels as a column of the dataset (the expected answer).
function LabelDialog({
  file,
  imageColumns,
  initialColumn,
  onClose,
  onSaved,
}: {
  file: string;
  imageColumns: string[];
  initialColumn: string;
  onClose: () => void;
  onSaved: (column: string) => void;
}) {
  const toast = useToast();
  const data = useLoad(() => api.previewDataset(file, 100000), [file]);
  const [column, setColumn] = useState(initialColumn || t("resposta_esperada", "expected_answer"));
  const [labels, setLabels] = useState<string[] | null>(null);
  const [saving, setSaving] = useState(false);

  const rows = data.data?.rows ?? [];
  const existing = data.data?.columns ?? [];
  // Before the first edit, show what the chosen column already holds.
  const shown = labels ?? rows.map((r) => r[column.trim()] ?? "");
  const filled = shown.filter((v) => v.trim()).length;
  // Labels typed so far, offered as suggestions so a class is typed only once.
  const suggestions = Array.from(new Set(shown.map((v) => v.trim()).filter(Boolean))).sort();
  const imageColumn = imageColumns.find((c) => existing.includes(c)) ?? imageColumns[0];

  function setLabel(i: number, value: string) {
    const next = [...shown];
    next[i] = value;
    setLabels(next);
  }

  async function save() {
    setSaving(true);
    try {
      await api.setDatasetColumn(file, column.trim(), shown.map((v) => v.trim()));
      toast(t(`Rótulos salvos na coluna ${column.trim()}`, `Labels saved in the ${column.trim()} column`));
      onSaved(column.trim());
    } catch (e) {
      toast((e as Error).message, "err");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog title={t("Rotular imagens", "Label images")} onClose={onClose}>
      <div className="field">
        <label htmlFor="label-col">{t("Coluna do rótulo", "Label column")}</label>
        <input
          id="label-col"
          className="inp mono"
          list="label-col-options"
          value={column}
          onChange={(e) => {
            setColumn(e.target.value);
            if (!labels || labels.every((v) => !v.trim())) setLabels(null);
          }}
        />
        <datalist id="label-col-options">
          {existing.filter((c) => !imageColumns.includes(c)).map((c) => (
            <option key={c} value={c} />
          ))}
        </datalist>
        <span className="hint">
          {t(
            "Escreva a resposta certa de cada imagem (ex.: a classe). Ao salvar, essa coluna vira a resposta esperada do experimento, e os avaliadores comparam a resposta do modelo com ela.",
            "Write the right answer for each image (e.g. its class). When you save, this column becomes the experiment's expected answer, and evaluators compare the model's answer with it.",
          )}
        </span>
      </div>

      {data.error && <div className="banner err">{data.error}</div>}
      {data.loading && <span className="muted">{t("Carregando…", "Loading…")}</span>}

      {rows.length > 0 && (
        <>
          <div style={{ maxHeight: "50vh", overflowY: "auto", display: "flex", flexDirection: "column", gap: 10 }}>
            {rows.map((row, i) => (
              <div key={i} className="row" style={{ gap: 12, flexWrap: "nowrap" }}>
                <span className="mono muted" style={{ width: 28, flexShrink: 0 }}>
                  {i + 1}
                </span>
                {row[imageColumn] ? (
                  <img src={imageURL(row[imageColumn])} alt={row.arquivo || row[imageColumn]} title={row.arquivo || row[imageColumn]} className="thumb" style={{ flexShrink: 0 }} />
                ) : (
                  <span className="muted">—</span>
                )}
                <input
                  className="inp"
                  style={{ flex: "1 1 auto", minWidth: 0 }}
                  list="label-suggestions"
                  aria-label={t(`Rótulo da imagem ${i + 1}`, `Label for image ${i + 1}`)}
                  value={shown[i]}
                  onChange={(e) => setLabel(i, e.target.value)}
                />
              </div>
            ))}
            <datalist id="label-suggestions">
              {suggestions.map((s) => (
                <option key={s} value={s} />
              ))}
            </datalist>
          </div>
          <span className="hint">
            {t(`${filled} de ${rows.length} imagens rotuladas.`, `${filled} of ${rows.length} images labeled.`)}
          </span>
        </>
      )}

      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn" onClick={onClose}>
          {t("Cancelar", "Cancel")}
        </button>
        <button className="btn pri" disabled={saving || !column.trim() || rows.length === 0 || imageColumns.includes(column.trim())} onClick={save}>
          {t("Salvar rótulos", "Save labels")}
        </button>
      </div>
    </Dialog>
  );
}

function PasteDialog({ experimentId, onClose, onSaved }: { experimentId: string; onClose: () => void; onSaved: (file: string) => void }) {
  const toast = useToast();
  const [name, setName] = useState(t("textos colados", "pasted texts"));
  const [text, setText] = useState("");
  const count = text.split(/\n\s*\n/).filter((t) => t.trim()).length;

  async function save() {
    try {
      const res = await api.pasteDataset(name, text, experimentId);
      toast(t(`Salvo como data/${res.file}`, `Saved as data/${res.file}`));
      onSaved(res.file);
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }

  return (
    <Dialog title={t("Colar textos", "Paste texts")} onClose={onClose}>
      <div className="field">
        <label htmlFor="paste-name">{t("Nome do arquivo", "File name")}</label>
        <input id="paste-name" className="inp" value={name} onChange={(e) => setName(e.target.value)} />
      </div>
      <div className="field">
        <label htmlFor="paste-text">{t("Textos", "Texts")}</label>
        <textarea
          id="paste-text"
          className="inp"
          rows={10}
          placeholder={t("Primeiro input…\n\nSegundo input…", "First input…\n\nSecond input…")}
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
        <span className="hint">
          {t("Separe cada input com uma linha em branco. Eles ficam na coluna ", "Separate inputs with a blank line. They go into the column ")}
          <span className="mono">{"{input}"}</span>.{" "}
          {count > 0 && <strong>{count} input(s).</strong>}
        </span>
      </div>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn" onClick={onClose}>
          {t("Cancelar", "Cancel")}
        </button>
        <button className="btn pri" disabled={count === 0} onClick={save}>
          {t("Salvar", "Save")}
        </button>
      </div>
    </Dialog>
  );
}

// ---------- Run settings & estimate ----------

function RunSettingsSection({ exp, update }: { exp: Experiment; update: (e: Experiment) => void }) {
  const set = (key: keyof Experiment["run"], value: number) => update({ ...exp, run: { ...exp.run, [key]: value } });
  return (
    <section className="card sec">
      <h2 style={{ fontSize: 18 }}>{t("Execução", "Run")}</h2>
      <div className="field">
        <label htmlFor="e-rep">{t("Repetições por input", "Repetitions per input")}</label>
        <input id="e-rep" className="inp" type="number" min={1} max={20} value={exp.run.repetitions}
          onChange={(e) => set("repetitions", Math.max(1, Number(e.target.value)))} />
        <span className="hint">{t("Rodar mais de uma vez mostra o quanto as respostas variam.", "Running more than once shows how much the answers vary.")}</span>
      </div>
      <div className="field">
        <label htmlFor="e-con">{t("Chamadas em paralelo", "Parallel calls")}</label>
        <input id="e-con" className="inp" type="number" min={1} max={32} value={exp.run.concurrency}
          onChange={(e) => set("concurrency", Math.max(1, Number(e.target.value)))} />
        <span className="hint">{t("Diminua se aparecer erro de “limite de uso”.", "Lower it if you get “rate limit” errors.")}</span>
      </div>
      <div className="field">
        <label htmlFor="e-to">{t("Tempo limite por chamada (s)", "Timeout per call (s)")}</label>
        <input id="e-to" className="inp" type="number" min={10} value={exp.run.timeout_s}
          onChange={(e) => set("timeout_s", Math.max(10, Number(e.target.value)))} />
      </div>
    </section>
  );
}

function EstimateSection({ exp, onRun }: { exp: Experiment; onRun: () => void }) {
  // Re-estimate whenever the parts that affect cost change.
  const key = JSON.stringify([exp.models, exp.prompt, exp.dataset.file, exp.run.repetitions]);
  const est = useLoad(() => api.estimate(exp.id), [key]);
  const e = est.data?.estimate;

  return (
    <section className="card sec">
      <h2 style={{ fontSize: 18 }}>{t("Estimativa", "Estimate")}</h2>
      {est.data && !est.data.ready && <div className="banner warn small">{est.data.problem}</div>}
      {e && (
        <div>
          <div className="sum"><span className="muted">{t("Chamadas", "Calls")}</span><span className="mono">{fmtInt(e.calls)}</span></div>
          <div className="sum"><span className="muted">{t("Tokens de entrada", "Input tokens")}</span><span className="mono">~{fmtCompact(e.input_tokens)}</span></div>
          <div className="sum"><span className="muted">{t("Custo em APIs", "API cost")}</span><span className="mono">{fmtUSD(e.cost_usd)}</span></div>
          {e.models_without_price.length > 0 && (
            <div className="sum"><span className="muted">{t("Sem preço", "No price")}</span><span className="mono tiny">{e.models_without_price.join(", ")}</span></div>
          )}
        </div>
      )}
      <button className="btn pri" onClick={onRun}>
        {t("Rodar experimento", "Run experiment")}
      </button>
      <span className="hint">{t("Estimativa baseada no tamanho dos inputs. Modelos locais não têm custo.", "Estimate based on the size of the inputs. Local models are free.")}</span>
    </section>
  );
}
