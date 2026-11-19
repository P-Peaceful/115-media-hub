import type { Dispatch, SetStateAction } from "react";
import { useOutletContext } from "react-router";

import type { Library } from "./api";

export type IncrementalSettings = {
  interval: string;
  batchLimit: string;
  listening: boolean;
  includeDirectories: boolean;
};

export function useLibraryDetailContext() {
  return useOutletContext<{
    item: Library;
    settings: IncrementalSettings;
    setSettings: Dispatch<SetStateAction<IncrementalSettings>>;
  }>();
}

export function libraryDetailPath(id: string) {
  return `/libraries/${encodeURIComponent(id)}`;
}
