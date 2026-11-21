import { describe, expect, it } from "vitest";

import {
  canRegenerateStrmFile,
  createStrmDemoFiles,
  previewStrmCleanup,
  regenerateStrmFile,
} from "./strm-demo";

const demoFiles = createStrmDemoFiles(new Date("2026-11-21T10:00:00Z"));

function fileById(id: string) {
  const file = demoFiles.find((candidate) => candidate.id === id);
  if (!file) throw new Error(`Missing demo file ${id}.`);
  return file;
}

describe("STRM demo file maintenance", () => {
  it("includes files generated in the last 24 hours even near midnight", () => {
    const generatedAt = new Date(2026, 10, 21, 0, 5);
    const recentThreshold = generatedAt.getTime() - 86_400_000;
    const recent = createStrmDemoFiles(generatedAt).filter(
      (file) => Date.parse(file.generatedAt) >= recentThreshold,
    );

    expect(recent.map(({ id }) => id)).toEqual([
      "wandering-earth-2",
      "oppenheimer",
    ]);
  });

  it("counts only selected, explicitly removable abnormal files as released space", () => {
    const valid = fileById("wandering-earth-2");
    const missing = fileById("dune-2");
    const regeneratable = fileById("three-body-s01e02");
    const pending = fileById("three-body-s01e01");
    const removableFiles = [
      valid,
      missing,
      regeneratable,
      { ...pending, cleanupDisposition: "remove" as const },
      fileById("hero"),
    ];
    const preview = previewStrmCleanup(removableFiles, [
      pending.id,
      valid.id,
      regeneratable.id,
      missing.id,
      missing.id,
      "missing-review-item",
    ]);

    expect(preview).toEqual({
      libraries: ["movie", "tv"],
      total: 4,
      releaseBytes: missing.sizeBytes,
      removableIds: [missing.id],
      rows: [
        { kind: "keep", count: 2, bytes: valid.sizeBytes + pending.sizeBytes },
        { kind: "remove", count: 1, bytes: missing.sizeBytes },
        { kind: "regenerate", count: 1, bytes: regeneratable.sizeBytes },
      ],
    });
  });

  it("regenerates eligible files without mutating the previous check's findings", () => {
    const missing = fileById("dune-2");
    const broken = fileById("three-body-s01e02");
    const pending = fileById("three-body-s01e01");
    const generatedAt = "2026-11-21T10:00:00.000Z";

    expect(canRegenerateStrmFile(missing)).toBe(false);
    expect(canRegenerateStrmFile(broken)).toBe(true);
    expect(canRegenerateStrmFile(pending)).toBe(true);

    const regenerated = regenerateStrmFile(broken, generatedAt);
    expect(regenerated).toMatchObject({
      status: "valid",
      sourceStatus: "linked",
      playbackStatus: "playable",
      cleanupDisposition: "keep",
      reason: null,
      generatedAt,
      lastCheckedAt: generatedAt,
    });
    expect(broken).toMatchObject({
      status: "invalid",
      reason: "播放链接返回 404",
    });
  });
});
