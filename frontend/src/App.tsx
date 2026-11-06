// Routes. URLs are in Portuguese because people see them in the address bar.

import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { Sidebar } from "./components/Sidebar";
import ExperimentsPage from "./pages/ExperimentsPage";
import ExperimentPage from "./pages/ExperimentPage";
import EvaluatorsPage from "./pages/EvaluatorsPage";
import SettingsPage from "./pages/SettingsPage";

export default function App() {
  const { pathname } = useLocation();
  // "/experimentos/abc/resultados" -> "/experimentos/abc": switching tabs inside
  // an experiment keeps the same screen (and its pending auto-save).
  const screen = pathname.split("/").slice(0, 3).join("/");
  return (
    <div className="app">
      <Sidebar />
      <main className="main">
        {/* key: going to another screen clears a previous error */}
        <ErrorBoundary key={screen}>
          <Routes>
            <Route path="/" element={<ExperimentsPage />} />
            <Route path="/experimentos/:id/*" element={<ExperimentPage />} />
            <Route path="/avaliadores" element={<EvaluatorsPage />} />
            <Route path="/configuracoes" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </ErrorBoundary>
      </main>
    </div>
  );
}
