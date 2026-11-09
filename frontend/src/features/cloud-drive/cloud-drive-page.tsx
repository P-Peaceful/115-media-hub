import "./cloud-drive.css";

import {
  Button,
  Dropdown,
  Label,
  ListBox,
  Select,
  Spinner,
  Table,
} from "@heroui/react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowUp,
  CalendarDays,
  Copy,
  Database,
  File,
  FileImage,
  FileText,
  Film,
  Folder,
  Grid2X2,
  Home,
  Layers,
  List,
  MoreHorizontal,
  RefreshCw,
  Search,
  X,
} from "lucide-react";
import { useState } from "react";
import { Link, useLocation, useSearchParams } from "react-router";

import folderArtwork from "@/assets/media-folder.svg";
import { formatBytes } from "@/features/settings/account-details";
import { ApiError } from "@/lib/api/errors";

import type { DriveItem } from "./api";
import { getDirectory } from "./api";

const filters = [
  ["all", "全部类型"],
  ["video", "视频文件"],
  ["audio", "音频文件"],
  ["image", "图片文件"],
  ["document", "文档文件"],
] as const;
const sorts = [
  ["name", "名称"],
  ["size", "大小"],
  ["modified_at", "时间"],
] as const;

function fileLabel(item: DriveItem) {
  if (item.is_directory) return "文件夹";
  const labels: Record<string, string> = {
    video: "视频文件",
    audio: "音频文件",
    image: "图片文件",
    document: "文档文件",
    file: "其他文件",
    subtitle: "字幕文件",
  };
  return labels[item.file_type] ?? (item.file_type || "未知类型");
}
function dateLabel(value: string | null) {
  if (!value) return "暂不可用";
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? "暂不可用"
    : new Intl.DateTimeFormat("zh-CN", {
        dateStyle: "short",
        timeStyle: "short",
      }).format(date);
}
function FileIcon({ item }: { item: DriveItem }) {
  const Icon = item.is_directory
    ? Folder
    : item.file_type === "video"
      ? Film
      : item.file_type === "image"
        ? FileImage
        : item.file_type === "document"
          ? FileText
          : File;
  return (
    <Icon
      aria-hidden="true"
      className={`drive-file-icon ${item.is_directory ? "folder" : item.file_type}`}
    />
  );
}
function ItemActions({
  item,
  onDetails,
  onCopy,
}: {
  item: DriveItem;
  onDetails: () => void;
  onCopy: () => void;
}) {
  return (
    <Dropdown>
      <Button
        variant="ghost"
        size="sm"
        isIconOnly
        aria-label={`${item.name} 的操作`}
      >
        <MoreHorizontal />
      </Button>
      <Dropdown.Popover>
        <Dropdown.Menu
          onAction={(key) => {
            if (key === "details") onDetails();
            if (key === "copy") onCopy();
          }}
        >
          <Dropdown.Item id="details" textValue="查看详情">
            <Label>查看详情</Label>
          </Dropdown.Item>
          <Dropdown.Item id="copy" textValue="复制路径">
            <Label>复制路径</Label>
          </Dropdown.Item>
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  );
}
function ErrorState({ error, retry }: { error: unknown; retry: () => void }) {
  const code = error instanceof ApiError ? error.code : "unknown_error";
  const authorization =
    code === "provider_not_configured" || code === "provider_auth_invalid";
  const message =
    code === "provider_not_configured"
      ? "尚未授权 115 网盘"
      : code === "provider_auth_invalid"
        ? "115 网盘授权已失效"
        : code === "not_found"
          ? "当前目录不存在或已被移除"
          : "无法加载目录，请检查网络后重试";
  return (
    <div className="drive-state" role="alert">
      <Folder aria-hidden="true" />
      <strong>{message}</strong>
      <p>
        {authorization
          ? "请前往设置完成网盘授权。"
          : "请稍后重试，或返回其他目录。"}
      </p>
      {authorization ? (
        <Link to="/settings?tab=account">管理网盘授权</Link>
      ) : (
        <Button variant="secondary" onPress={retry}>
          重试
        </Button>
      )}
    </div>
  );
}
export function CloudDrivePage() {
  const location = useLocation();
  const [view, setView] = useState<"list" | "grid">("list");
  return <CloudDriveContent key={location.key} view={view} setView={setView} />;
}
function CloudDriveContent({
  view,
  setView,
}: {
  view: "list" | "grid";
  setView: (view: "list" | "grid") => void;
}) {
  const [params, setParams] = useSearchParams();
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<DriveItem | null>(null);
  const [notice, setNotice] = useState("");
  const directory = params.get("directory_id") || "0";
  const sort = sorts.some(([value]) => value === params.get("sort"))
    ? params.get("sort") || "name"
    : "name";
  const order = params.get("order") === "desc" ? "desc" : "asc";
  const type = filters.some(([value]) => value === params.get("type"))
    ? params.get("type") || "all"
    : "all";
  const cursor = params.get("cursor") || "";
  const queryParams = new URLSearchParams({
    directory_id: directory,
    sort,
    order,
    type,
    limit: "50",
  });
  if (cursor) queryParams.set("cursor", cursor);
  const query = useQuery({
    queryKey: ["cloud-drive", directory, sort, order, type, cursor],
    queryFn: ({ signal }) => getDirectory(queryParams, signal),
    retry: false,
  });
  const listing = query.data;
  function change(values: Record<string, string | null>) {
    const next = new URLSearchParams(params);
    for (const [key, value] of Object.entries(values)) {
      if (value) next.set(key, value);
      else next.delete(key);
    }
    setSelected(null);
    setNotice("");
    setParams(next);
  }
  async function refresh() {
    setSelected(null);
    setNotice("");
    if (!cursor) {
      await query.refetch();
      return;
    }
    await queryClient.invalidateQueries({
      queryKey: ["cloud-drive", directory, sort, order, type, ""],
      exact: true,
      refetchType: "none",
    });
    change({ cursor: null });
  }
  function navigateDirectory(id: string) {
    change({ directory_id: id === "0" ? null : id, cursor: null });
  }
  function pathParts(item: DriveItem) {
    return [...(listing?.path.map((part) => part.name) ?? []), item.name];
  }
  async function copy(item: DriveItem) {
    try {
      await navigator.clipboard.writeText(
        `/${pathParts(item).slice(1).join("/")}`,
      );
      setNotice("路径已复制");
    } catch {
      setNotice("复制失败，请检查浏览器权限");
    }
  }
  const actions = (item: DriveItem) => (
    <ItemActions
      item={item}
      onDetails={() => setSelected(item)}
      onCopy={() => {
        void copy(item);
      }}
    />
  );
  return (
    <main className="drive-page">
      <div className={`drive-layout${selected ? " has-detail" : ""}`}>
        <div className="drive-content">
          <section className="drive-banner">
            <div>
              <h1>网盘浏览</h1>
              <p>浏览已授权的 115 网盘目录，查看文件、管理媒体库关联状态。</p>
            </div>
            <img src={folderArtwork} alt="" />
          </section>
          <nav className="drive-path" aria-label="目录路径">
            <Home aria-hidden="true" />
            {listing?.path.map((part, index) => (
              <span key={part.id}>
                {index > 0 && <span className="drive-separator">/</span>}
                <Button
                  variant="ghost"
                  size="sm"
                  onPress={() => navigateDirectory(part.id)}
                >
                  {part.name}
                </Button>
              </span>
            ))}
          </nav>
          <div className="drive-toolbar">
            <Button
              variant="secondary"
              size="sm"
              isDisabled={!listing || listing.path.length < 2}
              onPress={() => {
                const parent = listing?.path.at(-2);
                if (parent) navigateDirectory(parent.id);
              }}
            >
              <ArrowUp />
              返回上级
            </Button>
            <div className="drive-search" title="当前目录搜索暂不可用">
              <Search aria-hidden="true" />
              <input
                disabled
                aria-label="当前目录搜索暂不可用"
                placeholder="当前目录搜索暂不可用"
              />
            </div>
            <Button
              variant="secondary"
              size="sm"
              isPending={query.isFetching}
              onPress={() => {
                void refresh();
              }}
            >
              <RefreshCw />
              刷新
            </Button>
            <div className="drive-view">
              <Button
                variant={view === "list" ? "secondary" : "ghost"}
                size="sm"
                isIconOnly
                aria-label="列表视图"
                aria-pressed={view === "list"}
                onPress={() => setView("list")}
              >
                <List />
              </Button>
              <Button
                variant={view === "grid" ? "secondary" : "ghost"}
                size="sm"
                isIconOnly
                aria-label="网格视图"
                aria-pressed={view === "grid"}
                onPress={() => setView("grid")}
              >
                <Grid2X2 />
              </Button>
            </div>
          </div>
          <div className="drive-filters">
            {sorts.map(([value, label]) => (
              <Button
                key={value}
                variant="secondary"
                size="sm"
                aria-label={`${label}排序${sort === value ? (order === "asc" ? "，升序" : "，降序") : ""}`}
                onPress={() =>
                  change({
                    sort: value,
                    order: sort === value && order === "asc" ? "desc" : "asc",
                    cursor: null,
                  })
                }
              >
                {label} {sort === value ? (order === "asc" ? "↑" : "↓") : "↕"}
              </Button>
            ))}
            <Select
              aria-label="文件类型"
              selectedKey={type}
              onSelectionChange={(key) => {
                if (typeof key === "string")
                  change({ type: key, cursor: null });
              }}
            >
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {filters.map(([value, label]) => (
                    <ListBox.Item key={value} id={value} textValue={label}>
                      {label}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
          </div>
          <div className="drive-list">
            {query.isPending ? (
              <div className="drive-state" role="status">
                <Spinner />
                <strong>正在加载目录内容…</strong>
              </div>
            ) : query.isError ? (
              <ErrorState
                error={query.error}
                retry={() => {
                  void query.refetch();
                }}
              />
            ) : !listing ? null : listing.items.length === 0 ? (
              <div className="drive-state">
                <Folder aria-hidden="true" />
                <strong>
                  {cursor
                    ? "当前页没有文件"
                    : type === "all"
                      ? "当前目录为空"
                      : "没有符合条件的文件"}
                </strong>
                <p>
                  {type === "all"
                    ? "当前目录没有文件或文件夹。"
                    : "试试选择其他文件类型。"}
                </p>
              </div>
            ) : view === "list" ? (
              <Table variant="secondary">
                <Table.ScrollContainer>
                  <Table.Content aria-label="网盘目录">
                    <Table.Header>
                      <Table.Column isRowHeader>名称</Table.Column>
                      <Table.Column>关联媒体库</Table.Column>
                      <Table.Column>大小</Table.Column>
                      <Table.Column>修改时间</Table.Column>
                      <Table.Column>文件类型</Table.Column>
                      <Table.Column aria-label="操作"> </Table.Column>
                    </Table.Header>
                    <Table.Body>
                      {listing.items.map((item) => (
                        <Table.Row
                          key={item.id}
                          id={item.id}
                          className={
                            selected?.id === item.id ? "drive-selected" : ""
                          }
                        >
                          <Table.Cell>
                            <Button
                              className="drive-name"
                              variant="ghost"
                              onPress={() =>
                                item.is_directory
                                  ? navigateDirectory(item.id)
                                  : setSelected(item)
                              }
                            >
                              <FileIcon item={item} />
                              <span>{item.name}</span>
                            </Button>
                          </Table.Cell>
                          <Table.Cell>
                            <span className="drive-unavailable">暂不可用</span>
                          </Table.Cell>
                          <Table.Cell>
                            {item.is_directory
                              ? "—"
                              : formatBytes(item.size_bytes)}
                          </Table.Cell>
                          <Table.Cell>{dateLabel(item.modified_at)}</Table.Cell>
                          <Table.Cell>{fileLabel(item)}</Table.Cell>
                          <Table.Cell>{actions(item)}</Table.Cell>
                        </Table.Row>
                      ))}
                    </Table.Body>
                  </Table.Content>
                </Table.ScrollContainer>
              </Table>
            ) : (
              <div className="drive-grid">
                {listing.items.map((item) => (
                  <div className="drive-tile" key={item.id}>
                    <Button
                      variant="ghost"
                      className="drive-tile-name"
                      onPress={() =>
                        item.is_directory
                          ? navigateDirectory(item.id)
                          : setSelected(item)
                      }
                    >
                      <FileIcon item={item} />
                      <span>{item.name}</span>
                    </Button>
                    <span>{fileLabel(item)}</span>
                    {actions(item)}
                  </div>
                ))}
              </div>
            )}
            {listing && !query.isError && (cursor || listing.next_cursor) && (
              <div className="drive-pagination">
                {cursor && (
                  <Button
                    variant="secondary"
                    size="sm"
                    onPress={() => change({ cursor: null })}
                  >
                    返回第一页
                  </Button>
                )}
                {listing.next_cursor && (
                  <Button
                    variant="secondary"
                    size="sm"
                    onPress={() => change({ cursor: listing.next_cursor })}
                  >
                    下一页
                  </Button>
                )}
              </div>
            )}
          </div>
          <p className="drive-notice" role="status">
            {notice}
          </p>
        </div>
        {selected && (
          <aside className="drive-detail" aria-label="文件详情">
            <div className="drive-detail-heading">
              <h2>文件详情</h2>
              <Button
                variant="ghost"
                size="sm"
                isIconOnly
                aria-label="关闭文件详情"
                onPress={() => setSelected(null)}
              >
                <X />
              </Button>
            </div>
            <div className="drive-detail-file">
              <FileIcon item={selected} />
              <div>
                <strong>{selected.name}</strong>
                <span>{fileLabel(selected)}</span>
              </div>
            </div>
            <dl>
              <div>
                <dt>
                  <Folder />
                  完整路径
                </dt>
                <dd>{pathParts(selected).join(" / ")}</dd>
              </div>
              <div>
                <dt>
                  <Database />
                  大小
                </dt>
                <dd>
                  {selected.is_directory
                    ? "—"
                    : formatBytes(selected.size_bytes)}
                </dd>
              </div>
              <div>
                <dt>
                  <FileText />
                  类型
                </dt>
                <dd>{fileLabel(selected)}</dd>
              </div>
              <div>
                <dt>
                  <CalendarDays />
                  修改时间
                </dt>
                <dd>{dateLabel(selected.modified_at)}</dd>
              </div>
              <div>
                <dt>
                  <Layers />
                  所属媒体库
                </dt>
                <dd>
                  <span className="drive-unavailable">暂不可用</span>
                </dd>
              </div>
            </dl>
            <div className="drive-detail-actions">
              <Button
                variant="secondary"
                onPress={() => {
                  void copy(selected);
                }}
              >
                <Copy />
                复制路径
              </Button>
              <Button isDisabled>
                <Layers />
                跳转媒体库
              </Button>
            </div>
          </aside>
        )}
      </div>
    </main>
  );
}
