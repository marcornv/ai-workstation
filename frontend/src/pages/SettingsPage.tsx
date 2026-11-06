// Screen 5: Configurações — providers & keys, bancada.yaml editor, services status.

import { useState } from "react";
import { api, type ProviderKind, type ProviderView } from "../api";
import { Dialog } from "../components/Dialog";
import { useToast } from "../components/Toast";
import { providerKindLabel, providerTag } from "../format";
import { useLoad } from "../hooks";
import { t } from "../i18n";

type TestResult = { ok: boolean; ms?: number; error?: string };

export default function SettingsPage() {
  const toast = useToast();
  const settings = useLoad(() => api.settings(), []);
  const health = useLoad(() => api.health(), []);
  const [tests, setTests] = useState<Record<string, TestResult | "testing">>({});
  const [keyFor, setKeyFor] = useState<ProviderView | null>(null);
  const [editing, setEditing] = useState<ProviderView | "new" | null>(null);
  const [removing, setRemoving] = useState<ProviderView | null>(null);
  const [yaml, setYaml] = useState<string | null>(null);

  const s = settings.data;
  const yamlValue = yaml ?? s?.file ?? "";
  const ollama = s?.providers.find((p) => p.kind === "ollama");

  async function test(p: ProviderView) {
    setTests((t) => ({ ...t, [p.name]: "testing" }));
    const result = await api.testProvider(p.name).catch((e) => ({ ok: false, error: (e as Error).message }));
    setTests((t) => ({ ...t, [p.name]: result }));
  }

  async function saveYaml() {
    try {
      const next = await api.saveSettingsFile(yamlValue);
      settings.setData(next);
      setYaml(null);
      toast(t("bancada.yaml validado e salvo.", "bancada.yaml validated and saved."));
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }

  return (
    <>
      <header className="col-flex" style={{ gap: 6, maxWidth: 700 }}>
        <h1 style={{ fontSize: 32 }}>{t("Configurações", "Settings")}</h1>
        <p className="muted">
          {t("Tudo que está aqui vive em ", "Everything here lives in ")}
          <span className="mono">bancada.yaml</span>.{" "}
          {t(
            "Você pode editar o arquivo ou usar a interface; os dois sempre contam a mesma história.",
            "You can edit the file or use the interface; both always tell the same story.",
          )}
        </p>
      </header>

      <LanguageSection current={s?.config.language ?? "pt-BR"} />

      {settings.error && <div className="banner err">{settings.error}</div>}

      <section className="card sec">
        <div className="sechead">
          <h2>{t("Provedores e chaves", "Providers and keys")}</h2>
          <button className="btn sm" onClick={() => setEditing("new")}>
            {t("Adicionar provedor", "Add provider")}
          </button>
        </div>
        <div className="col-flex" style={{ gap: 10 }}>
          {(s?.providers ?? []).map((p) => {
            const check = tests[p.name];
            return (
              <div key={p.name} className="prow">
                <div className="row" style={{ gap: 12, flexWrap: "nowrap" }}>
                  <span className="prov" aria-hidden="true">{providerTag[p.kind]}</span>
                  <div>
                    <div style={{ fontWeight: 500 }}>{p.name}</div>
                    <div className="muted tiny">{providerKindLabel[p.kind]}</div>
                  </div>
                </div>
                <div>
                  {p.api_key_env ? (
                    <>
                      <div className="mono tiny">{p.api_key_env}</div>
                      <div className="muted tiny">{p.key_set ? t(`chave ${p.key_masked}`, `key ${p.key_masked}`) : t("chave não configurada", "no key set")}</div>
                    </>
                  ) : (
                    <>
                      <div className="mono tiny">{p.base_url}</div>
                      <div className="muted tiny">{t("servidor local, sem chave", "local server, no key")}</div>
                    </>
                  )}
                </div>
                <div>
                  {check === "testing" ? (
                    <span className="pill run">{t("Testando…", "Testing…")}</span>
                  ) : check ? (
                    check.ok ? (
                      <span className="pill ok">{t("Conexão ok", "Connection ok")} · {check.ms} ms</span>
                    ) : (
                      <span className="pill err" title={check.error}>{t("Falhou", "Failed")}</span>
                    )
                  ) : p.api_key_env && !p.key_set ? (
                    <span className="pill warn">{t("Falta a chave", "Key missing")}</span>
                  ) : (
                    <span className="pill draft">{t("Configurado", "Configured")}</span>
                  )}
                  {check && check !== "testing" && !check.ok && <div className="tiny" style={{ color: "var(--err)", marginTop: 4 }}>{check.error}</div>}
                </div>
                <div className="row" style={{ gap: 8, justifyContent: "flex-end" }}>
                  <button className="btn sm" onClick={() => test(p)}>{t("Testar", "Test")}</button>
                  {p.api_key_env && <button className="btn sm" onClick={() => setKeyFor(p)}>{p.key_set ? t("Trocar chave", "Change key") : t("Definir chave", "Set key")}</button>}
                  <button className="btn sm" onClick={() => setEditing(p)}>{t("Editar", "Edit")}</button>
                  <button className="btn sm danger" onClick={() => setRemoving(p)}>{t("Remover", "Remove")}</button>
                </div>
              </div>
            );
          })}
        </div>
        {ollama && <OllamaModels provider={ollama} />}
        <span className="muted small">
          {t(
            "As chaves nunca são gravadas no bancada.yaml. Ele guarda só o nome da variável de ambiente. Chaves digitadas aqui ficam no arquivo local .env, que só o seu usuário pode ler.",
            "Keys are never written to bancada.yaml. It only stores the name of the environment variable. Keys typed here go into the local .env file, which only your user can read.",
          )}
        </span>
      </section>

      <div className="row" style={{ alignItems: "flex-start", gap: 24 }}>
        <section className="card sec" style={{ flex: "1 1 560px", minWidth: 0 }}>
          <div className="sechead">
            <h2>{t("Arquivo de configuração", "Configuration file")}</h2>
            <span className="mono muted tiny">./{s?.file_path}</span>
          </div>
          <label htmlFor="yaml" className="sr-only">{t("Conteúdo de bancada.yaml", "Contents of bancada.yaml")}</label>
          <textarea id="yaml" className="yaml" spellCheck={false} value={yamlValue} onChange={(e) => setYaml(e.target.value)} />
          <div className="row">
            <button className="btn pri" onClick={saveYaml} disabled={yaml === null}>{t("Validar e salvar", "Validate and save")}</button>
            <button className="btn" onClick={() => { setYaml(null); settings.reload(); }}>{t("Recarregar do disco", "Reload from disk")}</button>
            {yaml !== null && <span className="pill warn">{t("Alterações não salvas", "Unsaved changes")}</span>}
          </div>
          <span className="muted small">
            {t(
              "Aqui ficam preços por modelo (pricing, em US$ por 1 milhão de tokens), o modelo juiz padrão e as pastas. Mudar a porta exige reiniciar o Bancada.",
              "This holds prices per model (pricing, in US$ per 1 million tokens), the default judge model and the folders. Changing the port requires restarting Bancada.",
            )}
          </span>
        </section>

        <div className="col-flex" style={{ flex: "0 1 400px", minWidth: 0, width: "100%", gap: 24 }}>
          <section className="card sec">
            <h2 style={{ fontSize: 18 }}>{t("Serviços", "Services")}</h2>
            <div>
              <div className="svc">
                <div>
                  <div style={{ fontWeight: 500 }}>{t("API de experimentos", "Experiments API")}</div>
                  <div className="mono muted tiny">Go · :{health.data?.api_port}</div>
                </div>
                <span className="pill ok">Online</span>
              </div>
              <div className="svc">
                <div>
                  <div style={{ fontWeight: 500 }}>{t("Serviço de avaliação", "Evaluation service")}</div>
                  <div className="mono muted tiny">Python · {health.data?.evaluator_url}</div>
                </div>
                {health.data?.evaluator ? <span className="pill ok">Online</span> : <span className="pill err">Offline</span>}
              </div>
              <div className="svc">
                <div>
                  <div style={{ fontWeight: 500 }}>{t("Armazenamento", "Storage")}</div>
                  <div className="mono muted tiny">{health.data?.storage_path}</div>
                </div>
                {health.data?.storage ? <span className="pill ok">{t("Gravável", "Writable")}</span> : <span className="pill err">{t("Erro", "Error")}</span>}
              </div>
            </div>
            {health.data && !health.data.evaluator && (
              <span className="muted small">
                {t(
                  "Sem o serviço de avaliação você ainda pode rodar experimentos. Para avaliar, abra o Bancada com ./start.sh (ele instala o Python necessário).",
                  "Without the evaluation service you can still run experiments. To evaluate, open Bancada with ./start.sh (it installs the Python it needs).",
                )}
              </span>
            )}
          </section>
          <section className="card sec">
            <h2 style={{ fontSize: 18 }}>{t("Onde fica cada coisa", "Where things live")}</h2>
            <div>
              <div className="sum"><span>{t("Configuração geral", "General settings")}</span><span className="mono tiny">bancada.yaml</span></div>
              <div className="sum"><span>{t("Experimentos", "Experiments")}</span><span className="mono tiny">{s?.config.paths.experiments}/</span></div>
              <div className="sum"><span>{t("Avaliadores", "Evaluators")}</span><span className="mono tiny">{s?.config.paths.evaluators}/</span></div>
              <div className="sum"><span>{t("Dados", "Data")}</span><span className="mono tiny">{s?.config.paths.datasets}/</span></div>
              <div className="sum"><span>{t("Resultados", "Results")}</span><span className="mono tiny">{s?.config.paths.database}</span></div>
              <div className="sum"><span>{t("Chaves", "Keys")}</span><span className="mono tiny">.env</span></div>
            </div>
            <span className="muted small">{t("Os arquivos são a fonte da verdade: a interface sempre lê o que está no disco.", "The files are the source of truth: the interface always reads what is on disk.")}</span>
          </section>
        </div>
      </div>

      {keyFor && (
        <KeyDialog
          provider={keyFor}
          onClose={() => setKeyFor(null)}
          onSaved={(next) => { settings.setData(next); setKeyFor(null); toast(t("Chave salva.", "Key saved.")); }}
        />
      )}
      {editing && (
        <ProviderDialog
          provider={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={(next) => { settings.setData(next); setEditing(null); setYaml(null); toast(t("Provedor salvo.", "Provider saved.")); }}
        />
      )}
      {removing && (
        <Dialog title={t("Remover provedor?", "Remove provider?")} onClose={() => setRemoving(null)}>
          <p>
            “{removing.name}”
            {t(
              " sai do bancada.yaml. Experimentos que usam este provedor vão mostrar “Provedor não existe” até você adicioná-lo de novo. A chave no .env não é apagada.",
              " will be removed from bancada.yaml. Experiments that use this provider will show “Provider does not exist” until you add it again. The key in .env is not deleted.",
            )}
          </p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <button className="btn" onClick={() => setRemoving(null)}>{t("Cancelar", "Cancel")}</button>
            <button
              className="btn danger solid"
              onClick={async () => {
                const next = await api.deleteProvider(removing.name);
                settings.setData(next);
                setYaml(null);
                setRemoving(null);
              }}
            >
              {t("Remover", "Remove")}
            </button>
          </div>
        </Dialog>
      )}
    </>
  );
}

// Interface language. Saved in bancada.yaml ("language"); the page reloads to apply it.
function LanguageSection({ current }: { current: "pt-BR" | "en" }) {
  const toast = useToast();
  async function change(language: "pt-BR" | "en") {
    try {
      await api.setLanguage(language);
      location.reload();
    } catch (e) {
      toast((e as Error).message, "err");
    }
  }
  return (
    <section className="card sec">
      <div className="sechead">
        <h2>{t("Idioma", "Language")}</h2>
      </div>
      <div className="row">
        <label htmlFor="ui-language" className="sr-only">
          {t("Idioma da interface", "Interface language")}
        </label>
        <select id="ui-language" className="inp" style={{ maxWidth: 260 }} value={current}
          onChange={(e) => change(e.target.value as "pt-BR" | "en")}>
          <option value="pt-BR">Português (Brasil)</option>
          <option value="en">English</option>
        </select>
        <span className="hint">
          {t(
            "Vale para a interface, as mensagens de erro e os motivos dos avaliadores. Notas e motivos já calculados ficam no idioma em que foram gerados.",
            "Applies to the interface, error messages and evaluator reasons. Scores and reasons already computed stay in the language they were generated in.",
          )}
        </span>
      </div>
    </section>
  );
}

function OllamaModels({ provider }: { provider: ProviderView }) {
  const toast = useToast();
  const models = useLoad(() => api.providerModels(provider.name), [provider.name]);
  const [pullName, setPullName] = useState("");
  const [pulling, setPulling] = useState(false);

  // Same as "ollama pull <model>", for people using Docker or no terminal.
  async function pull() {
    setPulling(true);
    try {
      await api.pullModel(provider.name, pullName.trim());
      toast(t(`Modelo ${pullName.trim()} baixado.`, `Model ${pullName.trim()} downloaded.`));
      setPullName("");
      models.reload();
    } catch (e) {
      toast((e as Error).message, "err");
    } finally {
      setPulling(false);
    }
  }

  return (
    <div className="card soft col-flex" style={{ padding: "14px 16px" }}>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <span>
          {models.error ? (
            <>{t("Ollama não encontrado em ", "Ollama not found at ")}<span className="mono">{provider.base_url}</span>. {t("Ele está instalado e aberto?", "Is it installed and running?")}</>
          ) : models.data ? (
            models.data.models?.length ? (
              <>{t("Modelos encontrados no Ollama: ", "Models found in Ollama: ")}{models.data.models.map((m, i) => (
                <span key={m}>{i > 0 && ", "}<span className="mono">{m}</span></span>
              ))}</>
            ) : (
              <>{t("O Ollama está rodando, mas ainda não tem modelos. Baixe um logo abaixo (ex.: ", "Ollama is running but has no models yet. Download one right below (e.g. ")}<span className="mono">llama3.1:8b</span>).</>
            )
          ) : (
            t("Procurando o Ollama…", "Looking for Ollama…")
          )}
        </span>
        <button className="btn sm" onClick={models.reload}>{t("Atualizar lista", "Refresh list")}</button>
      </div>
      {!models.error && (
        <div className="row" style={{ gap: 8 }}>
          <label htmlFor="pull-model" className="small" style={{ fontWeight: 500 }}>{t("Baixar modelo", "Download model")}</label>
          <input id="pull-model" className="inp mono" style={{ maxWidth: 260 }} placeholder={t("ex.: llama3.1:8b ou gemma3:4b", "e.g. llama3.1:8b or gemma3:4b")}
            value={pullName} onChange={(e) => setPullName(e.target.value)} disabled={pulling} />
          <button className="btn sm" onClick={pull} disabled={pulling || !pullName.trim()}>
            {pulling ? t("Baixando… (pode levar alguns minutos)", "Downloading… (this can take a few minutes)") : t("Baixar", "Download")}
          </button>
          <a className="btn link small" href="https://ollama.com/library" target="_blank" rel="noreferrer">{t("Ver modelos disponíveis", "See available models")}</a>
        </div>
      )}
    </div>
  );
}

function KeyDialog({ provider, onClose, onSaved }: { provider: ProviderView; onClose: () => void; onSaved: (s: Awaited<ReturnType<typeof api.settings>>) => void }) {
  const toast = useToast();
  const [key, setKey] = useState("");
  return (
    <Dialog title={t(`Chave de API · ${provider.name}`, `API key · ${provider.name}`)} onClose={onClose}>
      <div className="field">
        <label htmlFor="api-key">{t("Cole a chave", "Paste the key")}</label>
        <input id="api-key" className="inp mono" type="password" autoComplete="off" autoFocus value={key} onChange={(e) => setKey(e.target.value)} />
        <span className="hint">
          {t("Fica salva em .env como ", "It is saved in .env as ")}
          <span className="mono">{provider.api_key_env}</span>.{" "}
          {t(
            "Ela nunca é mostrada de novo nem enviada para outro lugar além do próprio provedor.",
            "It is never shown again or sent anywhere other than the provider itself.",
          )}
        </span>
      </div>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn" onClick={onClose}>{t("Cancelar", "Cancel")}</button>
        <button
          className="btn pri"
          disabled={!key.trim()}
          onClick={async () => {
            try {
              onSaved(await api.saveKey(provider.name, key));
            } catch (e) {
              toast((e as Error).message, "err");
            }
          }}
        >
          {t("Salvar chave", "Save key")}
        </button>
      </div>
    </Dialog>
  );
}

const defaultEnv: Record<ProviderKind, string> = {
  openai: "OPENAI_API_KEY",
  anthropic: "ANTHROPIC_API_KEY",
  gemini: "GEMINI_API_KEY",
  ollama: "",
  openai_compatible: "",
  typesafe: "TYPESAFE_API_KEY",
};
const defaultURL: Record<ProviderKind, string> = {
  openai: "https://api.openai.com",
  anthropic: "https://api.anthropic.com",
  gemini: "https://generativelanguage.googleapis.com",
  ollama: "http://localhost:11434",
  openai_compatible: "http://localhost:8000",
  typesafe: "https://api.typesafe.ai",
};

function ProviderDialog({ provider, onClose, onSaved }: { provider: ProviderView | null; onClose: () => void; onSaved: (s: Awaited<ReturnType<typeof api.settings>>) => void }) {
  const toast = useToast();
  const [name, setName] = useState(provider?.name ?? "");
  const [kind, setKind] = useState<ProviderKind>(provider?.kind ?? "openai_compatible");
  const [env, setEnv] = useState(provider?.api_key_env ?? "");
  const [url, setUrl] = useState(provider?.base_url ?? defaultURL.openai_compatible);

  return (
    <Dialog title={provider ? t(`Editar ${provider.name}`, `Edit ${provider.name}`) : t("Adicionar provedor", "Add provider")} onClose={onClose}>
      <div className="field">
        <label htmlFor="pv-name">{t("Nome", "Name")}</label>
        <input id="pv-name" className="inp mono" value={name} disabled={!!provider} placeholder={t("ex.: groq, lm-studio", "e.g. groq, lm-studio")}
          onChange={(e) => setName(e.target.value.toLowerCase())} />
        <span className="hint">{t("Letras minúsculas, números e hífen. É como os experimentos se referem a ele.", "Lowercase letters, numbers and hyphens. It is how experiments refer to it.")}</span>
      </div>
      <div className="field">
        <label htmlFor="pv-kind">{t("Tipo", "Type")}</label>
        <select id="pv-kind" className="inp" value={kind} onChange={(e) => {
          const k = e.target.value as ProviderKind;
          setKind(k);
          setEnv(defaultEnv[k]);
          setUrl(defaultURL[k]);
        }}>
          {Object.entries(providerKindLabel).map(([k, label]) => <option key={k} value={k}>{label}</option>)}
        </select>
      </div>
      <div className="field">
        <label htmlFor="pv-url">{t("Endereço (URL base)", "Address (base URL)")}</label>
        <input id="pv-url" className="inp mono" value={url} onChange={(e) => setUrl(e.target.value)} />
      </div>
      <div className="field">
        <label htmlFor="pv-env">{t("Variável de ambiente da chave", "Environment variable for the key")}</label>
        <input id="pv-env" className="inp mono" value={env} placeholder={t("vazio = sem chave", "empty = no key")} onChange={(e) => setEnv(e.target.value.toUpperCase())} />
      </div>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn" onClick={onClose}>{t("Cancelar", "Cancel")}</button>
        <button
          className="btn pri"
          disabled={!name.trim()}
          onClick={async () => {
            try {
              onSaved(await api.saveProvider(name.trim(), { kind, api_key_env: env.trim(), base_url: url.trim() }));
            } catch (e) {
              toast((e as Error).message, "err");
            }
          }}
        >
          {t("Salvar", "Save")}
        </button>
      </div>
    </Dialog>
  );
}
