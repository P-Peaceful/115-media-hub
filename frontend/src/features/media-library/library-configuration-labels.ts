import type { LibraryInput } from "./api";

export const mediaTypeLabels: Record<LibraryInput["media_type"], string> = {
  movie: "电影",
  tv: "电视剧",
  anime: "动漫",
  documentary: "纪录片",
  mixed: "混合",
};

export const languageLabels: Record<
  LibraryInput["language_preference"],
  string
> = {
  "zh-CN": "简体中文",
};

export const scrapeSourceLabels: Record<LibraryInput["scrape_source"], string> =
  {
    tmdb: "TMDB",
    metatube: "MetaTube",
    none: "不刮削",
  };

export const existingFilePolicyLabels: Record<
  LibraryInput["strm_existing_file_policy"],
  string
> = {
  skip: "跳过",
  overwrite: "覆盖",
};

export const linkTypeLabels: Record<LibraryInput["link_type"], string> = {
  direct: "直连",
  proxy: "302 代理",
};

export const linkFormatLabels: Record<
  LibraryInput["strm_link_format"],
  string
> = {
  original_name: "文件原名",
  file_id: "文件 ID",
};
