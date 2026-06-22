import { useCallback, useEffect, useReducer, useRef } from "react";
import { listRepositoryBranches } from "@/lib/api/facade/repositoryConnect";
import { readCurrentOrg } from "@/stores/auth";

type Status = "idle" | "loading" | "done" | "error";

interface CacheEntry {
  branches: string[];
  status: Status;
  reqToken: number;
}

const cache = new Map<number, CacheEntry>();
const inflight = new Map<number, Promise<void>>();
const listeners = new Map<number, Set<() => void>>();

function getEntry(id: number): CacheEntry {
  const existing = cache.get(id);
  if (existing) return existing;
  const entry: CacheEntry = { branches: [], status: "idle", reqToken: 0 };
  cache.set(id, entry);
  return entry;
}

function notify(id: number): void {
  listeners.get(id)?.forEach((fn) => fn());
}

function subscribe(id: number | null, cb: () => void): () => void {
  if (id === null) return () => undefined;
  const set = listeners.get(id) ?? new Set<() => void>();
  set.add(cb);
  listeners.set(id, set);
  return () => {
    const s = listeners.get(id);
    if (!s) return;
    s.delete(cb);
    if (s.size === 0) listeners.delete(id);
  };
}

function doFetch(id: number): void {
  if (inflight.has(id)) return;
  const entry = getEntry(id);
  const token = entry.reqToken + 1;
  entry.reqToken = token;
  entry.status = "loading";

  const orgSlug = readCurrentOrg()?.slug ?? "";
  const p = listRepositoryBranches(orgSlug, id)
    .then((res) => {
      const current = cache.get(id);
      if (!current || current.reqToken !== token) return;
      current.branches = res.items;
      current.status = "done";
      inflight.delete(id);
      notify(id);
    })
    .catch(() => {
      const current = cache.get(id);
      if (!current || current.reqToken !== token) return;
      current.status = "error";
      inflight.delete(id);
      notify(id);
    });
  inflight.set(id, p);
}

export interface UseRepositoryBranchesResult {
  branches: string[];
  loading: boolean;
  fallbackToFreeText: boolean;
  load: () => void;
  refresh: () => void;
}

export function useRepositoryBranches(repoId: number | null): UseRepositoryBranchesResult {
  const [, force] = useReducer((n: number) => n + 1, 0);
  const repoIdRef = useRef(repoId);
  repoIdRef.current = repoId;

  useEffect(() => {
    if (repoId === null) return;
    return subscribe(repoId, force);
  }, [repoId]);

  const load = useCallback(() => {
    const id = repoIdRef.current;
    if (id === null) return;
    const entry = getEntry(id);
    if (entry.status === "loading" || entry.status === "done") return;
    doFetch(id);
  }, []);

  const refresh = useCallback(() => {
    const id = repoIdRef.current;
    if (id === null) return;
    const entry = getEntry(id);
    if (entry.status === "loading") return;
    doFetch(id);
  }, []);

  if (repoId === null) {
    return { branches: [], loading: false, fallbackToFreeText: false, load, refresh };
  }

  const entry = getEntry(repoId);
  return {
    branches: entry.branches,
    loading: entry.status === "loading",
    fallbackToFreeText: entry.status === "error",
    load,
    refresh,
  };
}

export function __resetRepositoryBranchesCacheForTests(): void {
  cache.clear();
  inflight.clear();
  listeners.clear();
}
