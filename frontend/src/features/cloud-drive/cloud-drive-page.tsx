import "./cloud-drive.css";

import {
  Breadcrumbs,
  Button,
  Card,
  Dropdown,
  InputGroup,
  Label,
  Link as HeroLink,
  ListBox,
  Modal,
  Pagination,
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
} from "lucide-react";
import { Fragment, useRef, useState } from "react";
import { useLocation, useSearchParams } from "react-router";

import { PageBanner } from "@/components/page-banner";
import { formatBytes } from "@/features/settings/account-details";
import { ApiError } from "@/lib/api/errors";

import type { DirectoryListing, DriveItem } from "./api";
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
  onDetails: (trigger: HTMLButtonElement | null) => void;
  onCopy: () => void;
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  return (
    <Dropdown>
      <Button
        ref={trigger}
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
            if (key === "details") onDetails(trigger.current);
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
        <HeroLink href="/settings?tab=account">管理网盘授权</HeroLink>
      ) : (
        <Button variant="secondary" onPress={retry}>
          重试
        </Button>
      )}
    </div>
  );
}
export function CloudDrivePage() {
  const [view, setView] = useState<"list" | "grid">("list");
  return <CloudDriveContent view={view} setView={setView} />;
}
function CloudDriveContent({
  view,
  setView,
}: {
  view: "list" | "grid";
  setView: (view: "list" | "grid") => void;
}) {
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const queryClient = useQueryClient();
  const [selection, setSelection] = useState<{
    key: string;
    item: DriveItem | null;
  } | null>(null);
  const selected = selection?.key === location.key ? selection.item : null;
  const setSelected = (item: DriveItem | null) =>
    setSelection({ key: location.key, item });
  const detailTrigger = useRef<HTMLElement | null>(null);
  const [noticeState, setNoticeState] = useState<{
    key: string;
    message: string;
  } | null>(null);
  const notice = noticeState?.key === location.key ? noticeState.message : "";
  const setNotice = (message: string) =>
    setNoticeState({ key: location.key, message });
  const [lastLocationKey, setLastLocationKey] = useState(location.key);
  if (lastLocationKey !== location.key) {
    setLastLocationKey(location.key);
    setSelection(null);
    setNoticeState(null);
  }
  const directory = params.get("directory_id") || "0";
  const sort = sorts.some(([value]) => value === params.get("sort"))
    ? params.get("sort") || "name"
    : "name";
  const order = params.get("order") === "desc" ? "desc" : "asc";
  const type = filters.some(([value]) => value === params.get("type"))
    ? params.get("type") || "all"
    : "all";
  const cursor = params.get("cursor") || "";
  const rawPage = params.get("page") || "1";
  const parsedPage = Number(rawPage);
  const page =
    /^[1-9]\d*$/.test(rawPage) &&
    Number.isSafeInteger(parsedPage) &&
    parsedPage <= Math.floor(Number.MAX_SAFE_INTEGER / 50) + 1
      ? parsedPage
      : 1;
  const queryParams = new URLSearchParams({
    directory_id: directory,
    sort,
    order,
    type,
    limit: "50",
  });
  if (cursor) queryParams.set("cursor", cursor);
  else queryParams.set("page", String(page));
  const query = useQuery({
    queryKey: ["cloud-drive", directory, sort, order, type, cursor, page],
    queryFn: ({ signal }) => getDirectory(queryParams, signal),
    retry: false,
  });
  const listing = query.data;
  const cachedPaths = queryClient
    .getQueriesData<DirectoryListing>({ queryKey: ["cloud-drive"] })
    .sort(
      ([a], [b]) =>
        (queryClient.getQueryState(b)?.dataUpdatedAt ?? 0) -
        (queryClient.getQueryState(a)?.dataUpdatedAt ?? 0),
    );
  const pendingPath = query.isPending
    ? cachedPaths.flatMap(([, page]) => {
        if (!page) return [];
        const index = page.path.findIndex((part) => part.id === directory);
        if (index >= 0) return [page.path.slice(0, index + 1)];
        const child = page.items.find(
          (item) => item.id === directory && item.is_directory,
        );
        return child
          ? [[...page.path, { id: child.id, name: child.name }]]
          : [];
      })[0]
    : undefined;
  const path = query.isError ? undefined : (listing?.path ?? pendingPath);
  function change(values: Record<string, string | null>) {
    const next = new URLSearchParams(params);
    if (
      ["directory_id", "sort", "order", "type"].some((key) => key in values)
    ) {
      next.delete("page");
      next.delete("cursor");
    }
    if ("page" in values) next.delete("cursor");
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
    if (!cursor && page === 1) {
      await query.refetch();
      return;
    }
    await queryClient.invalidateQueries({
      queryKey: ["cloud-drive", directory, sort, order, type, "", 1],
      exact: true,
      refetchType: "none",
    });
    change({ cursor: null, page: null });
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
      onDetails={(trigger) => {
        detailTrigger.current = trigger;
        setSelected(item);
      }}
      onCopy={() => {
        void copy(item);
      }}
    />
  );
  const pageNumbers =
    listing?.page == null
      ? []
      : Array.from(
          new Set(
            [
              1,
              listing.total_pages,
              listing.page - 1,
              listing.page,
              listing.page + 1,
            ].filter((value) => value >= 1 && value <= listing.total_pages),
          ),
        ).sort((a, b) => a - b);
  const pagination =
    listing && !query.isError ? (
      <Pagination size="sm" aria-label="目录分页">
        <Pagination.Summary>
          {listing.page !== null && listing.items.length > 0 ? (
            <span>
              第 {(listing.page - 1) * listing.page_size + 1}–
              {(listing.page - 1) * listing.page_size + listing.items.length}{" "}
              项，
            </span>
          ) : null}
          <span>总计 {listing.total} 项</span>，
          <span>本页 {listing.items.length} 项</span>
          {listing.page !== null && listing.page > listing.total_pages && (
            <span>；目录已变化，当前页不可用</span>
          )}
        </Pagination.Summary>
        <Pagination.Content className="flex-wrap">
          {listing.page === null ||
          (cursor &&
            listing.next_cursor &&
            listing.items.length < listing.page_size) ? (
            <>
              <Pagination.Item>
                <Button
                  variant="secondary"
                  size="sm"
                  onPress={() => change({ cursor: null, page: null })}
                >
                  返回第一页
                </Button>
              </Pagination.Item>
              {listing.next_cursor && (
                <Pagination.Item>
                  <Pagination.Next
                    onPress={() =>
                      change({ cursor: listing.next_cursor, page: null })
                    }
                  >
                    下一页 <Pagination.NextIcon />
                  </Pagination.Next>
                </Pagination.Item>
              )}
            </>
          ) : (
            <>
              <Pagination.Item>
                <Pagination.Previous
                  isDisabled={listing.page <= 1}
                  onPress={() =>
                    change({
                      page: String(
                        Math.min((listing.page ?? 1) - 1, listing.total_pages),
                      ),
                    })
                  }
                >
                  <Pagination.PreviousIcon />
                  上一页
                </Pagination.Previous>
              </Pagination.Item>
              {pageNumbers.map((number, index) => (
                <Fragment key={number}>
                  {index > 0 &&
                    number - (pageNumbers[index - 1] ?? number) > 1 && (
                      <Pagination.Item>
                        <Pagination.Ellipsis />
                      </Pagination.Item>
                    )}
                  <Pagination.Item>
                    <Pagination.Link
                      aria-label={`第 ${number} 页`}
                      isActive={number === listing.page}
                      onPress={() =>
                        change({ page: number === 1 ? null : String(number) })
                      }
                    >
                      {number}
                    </Pagination.Link>
                  </Pagination.Item>
                </Fragment>
              ))}
              <Pagination.Item>
                <Pagination.Next
                  isDisabled={listing.page >= listing.total_pages}
                  onPress={() =>
                    change({ page: String((listing.page ?? 1) + 1) })
                  }
                >
                  下一页 <Pagination.NextIcon />
                </Pagination.Next>
              </Pagination.Item>
            </>
          )}
        </Pagination.Content>
      </Pagination>
    ) : null;
  return (
    <main className="drive-page">
      <PageBanner
        title="网盘浏览"
        description="浏览已授权的 115 网盘目录，查看文件、管理媒体库关联状态。"
      />
      <Card className="mt-4">
        <Card.Content>
          <div className="drive-content">
            <nav className="drive-path" aria-label="目录路径">
              {path ? (
                <Breadcrumbs separator="/">
                  {path.map((part, index) => {
                    const next = new URLSearchParams(params);
                    if (part.id === "0") next.delete("directory_id");
                    else next.set("directory_id", part.id);
                    next.delete("cursor");
                    next.delete("page");
                    const href = `?${next.toString()}`;
                    return (
                      <Breadcrumbs.Item
                        key={part.id}
                        {...(index === path.length - 1
                          ? {}
                          : {
                              href,
                              onClick: (event: React.MouseEvent) => {
                                if (
                                  event.metaKey ||
                                  event.ctrlKey ||
                                  event.shiftKey ||
                                  event.altKey ||
                                  event.button !== 0
                                )
                                  return;
                                event.preventDefault();
                                navigateDirectory(part.id);
                              },
                            })}
                      >
                        <span className="inline-flex items-center gap-2">
                          {index === 0 && (
                            <Home aria-hidden="true" className="size-4" />
                          )}
                          {part.name}
                        </span>
                      </Breadcrumbs.Item>
                    );
                  })}
                </Breadcrumbs>
              ) : (
                <span role="status">
                  {query.isError ? "目录路径暂不可用" : "正在加载目录路径…"}
                </span>
              )}
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
                <InputGroup className="w-full">
                  <InputGroup.Prefix>
                    <Search aria-hidden="true" />
                  </InputGroup.Prefix>
                  <InputGroup.Input
                    disabled
                    aria-label="当前目录搜索暂不可用"
                    placeholder="当前目录搜索暂不可用"
                  />
                </InputGroup>
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
                    {cursor || page > 1
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
                <Table>
                  <Table.ScrollContainer>
                    <Table.Content
                      aria-label="网盘目录"
                      className="min-w-[700px]"
                    >
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
                          <Table.Row key={item.id} id={item.id}>
                            <Table.Cell>
                              <Button
                                className="drive-name"
                                variant="ghost"
                                size="sm"
                                onPress={(event) => {
                                  if (item.is_directory)
                                    navigateDirectory(item.id);
                                  else {
                                    detailTrigger.current =
                                      event.target instanceof HTMLElement
                                        ? event.target
                                        : null;
                                    setSelected(item);
                                  }
                                }}
                              >
                                <FileIcon item={item} />
                                <span>{item.name}</span>
                              </Button>
                            </Table.Cell>
                            <Table.Cell>
                              <span className="drive-unavailable">
                                暂不可用
                              </span>
                            </Table.Cell>
                            <Table.Cell>
                              {item.is_directory
                                ? "—"
                                : formatBytes(item.size_bytes)}
                            </Table.Cell>
                            <Table.Cell>
                              {dateLabel(item.modified_at)}
                            </Table.Cell>
                            <Table.Cell>{fileLabel(item)}</Table.Cell>
                            <Table.Cell>{actions(item)}</Table.Cell>
                          </Table.Row>
                        ))}
                      </Table.Body>
                    </Table.Content>
                  </Table.ScrollContainer>
                  {pagination && <Table.Footer>{pagination}</Table.Footer>}
                </Table>
              ) : (
                <div className="drive-grid">
                  {listing.items.map((item) => (
                    <Card className="drive-tile" key={item.id}>
                      <Card.Content>
                        <Button
                          variant="ghost"
                          className="drive-tile-name"
                          onPress={(event) => {
                            if (item.is_directory) navigateDirectory(item.id);
                            else {
                              detailTrigger.current =
                                event.target instanceof HTMLElement
                                  ? event.target
                                  : null;
                              setSelected(item);
                            }
                          }}
                        >
                          <FileIcon item={item} />
                          <span>{item.name}</span>
                        </Button>
                        <span>{fileLabel(item)}</span>
                        {actions(item)}
                      </Card.Content>
                    </Card>
                  ))}
                </div>
              )}
              {(view === "grid" || !listing?.items.length) && pagination && (
                <div className="drive-pagination">{pagination}</div>
              )}
            </div>
            <p className="drive-notice" role="status">
              {notice}
            </p>
          </div>
        </Card.Content>
      </Card>
      {selected && (
        <Modal.Backdrop
          isOpen
          onOpenChange={(open) => {
            if (!open) {
              setSelected(null);
              requestAnimationFrame(() => detailTrigger.current?.focus());
            }
          }}
        >
          <Modal.Container>
            <Modal.Dialog>
              <Modal.CloseTrigger aria-label="关闭文件详情" />
              <Modal.Header>
                <Modal.Heading>文件详情</Modal.Heading>
              </Modal.Header>
              <Modal.Body>
                <div className="drive-detail">
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
                  {notice && <p role="status">{notice}</p>}
                </div>
              </Modal.Body>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      )}
    </main>
  );
}
