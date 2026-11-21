import { createContext, useContext } from "react";

import type { StrmFile } from "./strm-demo";

export type StrmView = "list" | "results";

export type StrmSelection = {
  filterKey: string;
  ids: ReadonlySet<string>;
};

export type StrmCheckRun = {
  files: readonly StrmFile[];
  checkedAt: string;
  durationMs: number;
  returnTo: string;
};

export type StrmCleanupScope = {
  fileIds: readonly string[];
  returnTo: string;
};

type StrmSession = {
  files: readonly StrmFile[];
  checkRun: StrmCheckRun | null;
  cleanup: StrmCleanupScope | null;
  selection: Record<StrmView, StrmSelection | null>;
  setSelection: (view: StrmView, selection: StrmSelection | null) => void;
  checkFiles: (ids: Iterable<string>, returnTo: string) => void;
  regenerateFiles: (ids: Iterable<string>) => number;
  openCleanup: (ids: Iterable<string>, returnTo: string) => void;
  cancelCleanup: () => void;
  confirmCleanup: () => number;
  feedback: string | null;
  setFeedback: (message: string | null) => void;
};

export const StrmSessionContext = createContext<StrmSession | null>(null);

export function useStrmSession() {
  const session = useContext(StrmSessionContext);
  if (!session) {
    throw new Error("STRM routes require a mounted STRM session.");
  }
  return session;
}
