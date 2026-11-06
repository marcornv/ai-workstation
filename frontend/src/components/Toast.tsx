// A small notification in the corner of the screen. Use: const toast = useToast(); toast("Salvo!")

import { createContext, useCallback, useContext, useState, type ReactNode } from "react";

type ShowToast = (message: string, kind?: "ok" | "err") => void;

const ToastContext = createContext<ShowToast>(() => {});

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<{ message: string; kind: "ok" | "err" } | null>(null);

  const show = useCallback<ShowToast>((message, kind = "ok") => {
    setToast({ message, kind });
    setTimeout(() => setToast((t) => (t?.message === message ? null : t)), kind === "err" ? 7000 : 3000);
  }, []);

  return (
    <ToastContext.Provider value={show}>
      {children}
      {toast && (
        <div className={`toast ${toast.kind}`} role="status" aria-live="polite">
          {toast.message}
        </div>
      )}
    </ToastContext.Provider>
  );
}

export const useToast = () => useContext(ToastContext);
