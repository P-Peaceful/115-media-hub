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
      className="relative flex min-h-[125px] items-center justify-between overflow-hidden rounded-2xl bg-[linear-gradient(110deg,#f0f0ff,#dbdafa_70%,#bfc7ff)] p-5 text-[#11152a] min-[701px]:min-h-[126px] min-[701px]:px-[34px] min-[701px]:py-6"
    >
      <div className="min-w-0">
        <h1
          data-page-banner-title
          className="mb-[7px] wrap-anywhere text-[clamp(24px,2.4vw,34px)] font-[650] tracking-[-0.03em]"
        >
          {title}
        </h1>
        <p className="m-0 text-[#555e7a]">{description}</p>
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
      <span
        aria-hidden="true"
        className="pointer-events-none absolute -top-20 -right-[30px] h-[290px] w-[400px] rotate-[35deg] bg-[linear-gradient(90deg,transparent,rgba(255,255,255,0.15),transparent)]"
      />
    </section>
  );
}
