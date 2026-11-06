// Experiment › Arquivo: edit experiments/<id>.yaml directly.

import { useState } from "react";
import { api, type Experiment } from "../api";
import { useToast } from "../components/Toast";
import { useLoad } from "../hooks";
import { t } from "../i18n";

export default function FileTab({ exp, onSaved }: { exp: Experiment; onSaved: () => void }) {
  const toast = useToast();
  // Reload the file whenever the experiment changes (e.g. auto-save from another tab).
  const file = useLoad(() => api.experimentFile(exp.id), [exp]);
  const [text, setText] = useState<string | null>(null);
  const value = text ?? file.data?.yaml ?? "";

  async function save() {
    try {
      await api.saveExperimentFile(exp.id, value);
      setText(null);
      toast(t("Arquivo validado e salvo.", "File validated and saved."));
      onSaved();
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }

  return (
    <section className="card sec">
      <div className="sechead">
        <h2>{t("Arquivo de configuração", "Configuration file")}</h2>
        <span className="muted mono tiny">{file.data?.path}</span>
      </div>
      <label htmlFor="exp-yaml" className="sr-only">
        {t("Conteúdo do arquivo", "File contents")}
      </label>
      <textarea id="exp-yaml" className="yaml" spellCheck={false} value={value} onChange={(e) => setText(e.target.value)} />
      <div className="row">
        <button className="btn pri" onClick={save} disabled={text === null}>
          {t("Validar e salvar", "Validate and save")}
        </button>
        <button className="btn" onClick={() => { setText(null); file.reload(); }}>
          {t("Recarregar do disco", "Reload from disk")}
        </button>
        {text !== null && <span className="pill warn">{t("Alterações não salvas", "Unsaved changes")}</span>}
      </div>
      <span className="muted small">
        {t(
          "Editar aqui ou na interface produz o mesmo arquivo. Mudanças feitas direto no arquivo aparecem na tela ao recarregar. Ao salvar pela interface, comentários do arquivo não são mantidos.",
          "Editing here or in the interface produces the same file. Changes made directly to the file show up when you reload. Saving from the interface does not keep comments in the file.",
        )}
      </span>
    </section>
  );
}
