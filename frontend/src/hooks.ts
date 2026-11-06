// hooks.ts: tiny helpers for loading data from the API.

import { useCallback, useEffect, useRef, useState } from "react";

// useLoad runs an async function on mount (and when deps change) and keeps
// its result, error and a reload() function.
export function useLoad<T>(load: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const loadRef = useRef(load);
  loadRef.current = load;

  const reload = useCallback(async () => {
    try {
      const value = await loadRef.current();
      setData(value);
      setError("");
      return value;
    } catch (e) {
      setError((e as Error).message);
      return null;
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    setLoading(true);
    reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { data, setData, error, loading, reload };
}

// useInterval calls fn every `ms` milliseconds while `active` is true.
export function useInterval(fn: () => void, ms: number, active: boolean) {
  const fnRef = useRef(fn);
  fnRef.current = fn;
  useEffect(() => {
    if (!active) return;
    const id = setInterval(() => fnRef.current(), ms);
    return () => clearInterval(id);
  }, [ms, active]);
}
