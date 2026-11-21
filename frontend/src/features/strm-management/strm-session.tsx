import { useState } from "react";
import { Outlet } from "react-router";

import {
  canRegenerateStrmFile,
  createStrmDemoFiles,
  previewStrmCleanup,
  regenerateStrmFile,
} from "./strm-demo";
import type {
  StrmCheckRun,
  StrmCleanupScope,
  StrmSelection,
  StrmView,
} from "./strm-session-context";
import { StrmSessionContext } from "./strm-session-context";

export function StrmSessionLayout() {
  const [files, setFiles] = useState(() => createStrmDemoFiles());
  const [checkRun, setCheckRun] = useState<StrmCheckRun | null>(null);
  const [cleanup, setCleanup] = useState<StrmCleanupScope | null>(null);
  const [selection, setSelections] = useState<
    Record<StrmView, StrmSelection | null>
  >({ list: null, results: null });
  const [feedback, setFeedback] = useState<string | null>(null);

  function setSelection(view: StrmView, next: StrmSelection | null) {
    setSelections((previous) => ({ ...previous, [view]: next }));
  }

  function checkFiles(ids: Iterable<string>, returnTo: string) {
    const startedAt = performance.now();
    const selectedIds = new Set(ids);
    const checkedAt = new Date().toISOString();
    const checkedFiles = files
      .filter((file) => selectedIds.has(file.id))
      .map((file) => ({ ...file, lastCheckedAt: checkedAt }));
    if (!checkedFiles.length) return;

    // The run keeps its findings even if the current files are later regenerated.
    setCheckRun({
      files: checkedFiles,
      checkedAt,
      durationMs: Math.max(1, Math.round(performance.now() - startedAt)),
      returnTo,
    });
    setFiles((previous) =>
      previous.map((file) =>
        selectedIds.has(file.id) ? { ...file, lastCheckedAt: checkedAt } : file,
      ),
    );
    setSelection("results", null);
    setFeedback(null);
  }

  function regenerateFiles(ids: Iterable<string>) {
    const selectedIds = new Set(ids);
    const eligibleIds = new Set(
      files
        .filter(
          (file) => selectedIds.has(file.id) && canRegenerateStrmFile(file),
        )
        .map((file) => file.id),
    );
    if (!eligibleIds.size) {
      setFeedback("没有可重新生成的演示文件。");
      return 0;
    }

    const generatedAt = new Date().toISOString();
    setFiles((previous) =>
      previous.map((file) =>
        eligibleIds.has(file.id) ? regenerateStrmFile(file, generatedAt) : file,
      ),
    );
    setFeedback(`已重新生成 ${eligibleIds.size} 个演示文件。`);
    return eligibleIds.size;
  }

  function openCleanup(ids: Iterable<string>, returnTo: string) {
    const selectedIds = new Set(ids);
    setCleanup({
      fileIds: files
        .filter((file) => selectedIds.has(file.id))
        .map(({ id }) => id),
      returnTo,
    });
  }

  function confirmCleanup() {
    if (!cleanup) return 0;

    const { removableIds } = previewStrmCleanup(files, cleanup.fileIds);
    const removed = new Set(removableIds);
    if (!removed.size) return 0;

    function pruneSelection(previous: StrmSelection | null) {
      if (previous === null) return null;
      const ids = new Set([...previous.ids].filter((id) => !removed.has(id)));
      return ids.size ? { ...previous, ids } : null;
    }

    setFiles((previous) => previous.filter((file) => !removed.has(file.id)));
    setSelections((previous) => ({
      list: pruneSelection(previous.list),
      results: pruneSelection(previous.results),
    }));
    setCleanup(null);
    setFeedback(`已清理 ${removed.size} 个演示文件。`);
    return removed.size;
  }

  return (
    <StrmSessionContext
      value={{
        files,
        checkRun,
        cleanup,
        selection,
        setSelection,
        checkFiles,
        regenerateFiles,
        openCleanup,
        cancelCleanup: () => setCleanup(null),
        confirmCleanup,
        feedback,
        setFeedback,
      }}
    >
      <Outlet />
    </StrmSessionContext>
  );
}
