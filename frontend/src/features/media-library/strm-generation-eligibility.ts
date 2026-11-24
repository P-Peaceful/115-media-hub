import type { Library } from "./api";

export function strmGenerationIneligibility(library: Library) {
  if (!library.enabled) return "请先启用此媒体库。";
  if (!library.generate_strm) return "请先在媒体库配置中启用 STRM 文件输出。";
  if (library.link_type !== "proxy") return "全量同步目前支持 302 代理链接。";
  if (!library.strm_direct_domain.trim())
    return "请先在媒体库配置中填写 STRM 播放地址。";
  return null;
}
