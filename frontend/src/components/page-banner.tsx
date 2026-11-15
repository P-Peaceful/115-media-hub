import type { ReactNode } from "react";

import folderArtwork from "@/assets/media-folder.svg";

export function PageBanner({
  title,
  description,
}: {
  title: ReactNode;
  description: ReactNode;
}) {
  return (
    <section
      data-slot="page-banner"
      className="page-banner relative flex min-h-[125px] items-center justify-between overflow-hidden rounded-2xl bg-surface-secondary p-5 text-foreground min-[701px]:min-h-[126px] min-[701px]:px-[34px] min-[701px]:py-6"
    >
      <div className="min-w-0">
        <h1
          data-page-banner-title
          className="mb-[7px] wrap-anywhere text-[clamp(24px,2.4vw,34px)] font-[650] tracking-[-0.03em]"
        >
          {title}
        </h1>
        <p className="m-0 text-foreground">{description}</p>
      </div>
      <div
        className="relative h-0 w-[90px] shrink-0 min-[701px]:w-[270px]"
        aria-hidden="true"
      >
        <img
          className="pointer-events-none absolute top-1/2 right-0 w-[90px] -translate-y-1/2 object-contain min-[701px]:h-[125px] min-[701px]:w-[270px]"
          src={folderArtwork}
          alt=""
        />
      </div>
    </section>
  );
}
