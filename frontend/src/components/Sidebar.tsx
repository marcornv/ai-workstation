import { NavLink, useLocation } from "react-router-dom";
import { api } from "../api";
import { useInterval, useLoad } from "../hooks";
import { language, t, type Language } from "../i18n";

export function Logo() {
  return (
    <svg width="26" height="26" viewBox="0 0 26 26" aria-hidden="true">
      <rect x="1" y="1" width="24" height="24" rx="6" fill="#0B6E73" />
      <path d="M7 18V12M13 18V8M19 18v-4" stroke="#fff" strokeWidth="2.2" strokeLinecap="round" />
    </svg>
  );
}

export function Sidebar() {
  const { pathname } = useLocation();
  const health = useLoad(() => api.health(), []);
  useInterval(health.reload, 15000, true);
  const h = health.data;

  return (
    <aside className="side">
      <div className="brand">
        <Logo />
        Bancada
      </div>
      <nav className="nav" aria-label={t("Principal", "Main")}>
        <div className="navlabel">Workspace</div>
        <NavLink to="/" end className={({ isActive }) => (isActive || pathname.startsWith("/experimentos") ? "active" : "")}>
          {t("Experimentos", "Experiments")}
        </NavLink>
        <NavLink to="/avaliadores">{t("Avaliadores", "Evaluators")}</NavLink>
        <NavLink to="/configuracoes">{t("Configurações", "Settings")}</NavLink>
      </nav>
      <div className="field" style={{ marginTop: "auto" }}>
        <label htmlFor="side-language" className="sr-only">
          {t("Idioma", "Language")}
        </label>
        <select id="side-language" className="inp" value={language()}
          onChange={async (e) => {
            await api.setLanguage(e.target.value as Language);
            location.reload();
          }}>
          <option value="pt-BR">Português</option>
          <option value="en">English</option>
        </select>
      </div>
      <div className="cfgstate" style={{ marginTop: 0 }}>
        <span>
          <span className={`dot ${h?.api ? "" : "off"}`} />
          bancada.yaml
        </span>
        <span>
          <span className={`dot ${h?.evaluator ? "" : "off"}`} />
          {t("avaliação", "evaluation")} {h?.evaluator ? "online" : "offline"}
        </span>
      </div>
    </aside>
  );
}
