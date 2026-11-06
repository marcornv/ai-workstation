// A modal dialog. Closes with Esc or a click outside.

import { useEffect, type ReactNode } from "react";

export function Dialog({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="card dialog" role="dialog" aria-modal="true" aria-label={title}>
        <h2 style={{ fontSize: 22 }}>{title}</h2>
        {children}
      </div>
    </div>
  );
}
