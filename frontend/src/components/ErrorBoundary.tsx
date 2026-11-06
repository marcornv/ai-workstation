// Safety net: if a screen crashes, show a message instead of a blank page.

import { Component, type ReactNode } from "react";
import { t } from "../i18n";

type Props = { children: ReactNode };
type State = { error: Error | null };

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <div className="card sec">
        <h2>{t("Algo deu errado nesta tela", "Something went wrong on this screen")}</h2>
        <p className="muted">
          {t(
            "Seus dados estão salvos. Tente recarregar a página. Se continuar, copie a mensagem abaixo ao pedir ajuda.",
            "Your data is saved. Try reloading the page. If it keeps happening, copy the message below when asking for help.",
          )}
        </p>
        <pre className="prompt-box">{this.state.error.message}</pre>
        <div className="row">
          <button className="btn pri" onClick={() => location.reload()}>{t("Recarregar", "Reload")}</button>
          <a className="btn" href="/">{t("Voltar para Experimentos", "Back to Experiments")}</a>
        </div>
      </div>
    );
  }
}
