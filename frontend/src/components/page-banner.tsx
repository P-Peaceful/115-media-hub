import type { ReactNode } from "react";

import folderArtwork from "@/assets/media-folder.svg";

export function PageBanner({
  title,
  description,
  compact = false,
}: {
  title: ReactNode;
  description: ReactNode;
  compact?: boolean;
}) {
  return (
    <section
      data-slot="page-banner"
      className={`page-banner relative flex items-center justify-between overflow-hidden rounded-2xl bg-surface-secondary text-foreground ${
        compact
          ? "min-h-24 px-4 py-3 sm:px-6"
          : "min-h-[125px] p-5 min-[701px]:min-h-[126px] min-[701px]:px-[34px] min-[701px]:py-6"
      }`}
    >
      <div className="min-w-0">
        <h1
          data-page-banner-title
          className={`wrap-anywhere tracking-[-0.03em] ${
            compact
              ? "mb-1 text-2xl font-bold sm:text-3xl"
              : "mb-[7px] text-[clamp(24px,2.4vw,34px)] font-[650]"
          }`}
        >
          {title}
        </h1>
        <p
          className={
            compact ? "m-0 text-sm text-foreground" : "m-0 text-foreground"
          }
        >
          {description}
        </p>
      </div>
      <div
        className={`relative h-0 shrink-0 ${compact ? "w-20 sm:w-52" : "w-[90px] min-[701px]:w-[270px]"}`}
        aria-hidden="true"
      >
        <img
          className={`pointer-events-none absolute top-1/2 right-0 w-full -translate-y-1/2 object-contain ${compact ? "sm:h-28" : "min-[701px]:h-[125px]"}`}
          src={folderArtwork}
          alt=""
        />
      </div>
    </section>
  );
}
