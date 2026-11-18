import {
  Alert,
  AlertDialog,
  Button,
  Card,
  Checkbox,
  Chip,
  FieldError,
  Form,
  Input,
  InputGroup,
  Label,
  Link as HeroLink,
  ListBox,
  Select,
  Switch,
  Tabs,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
} from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  FileText,
  Film,
  Folder,
  Layers,
  Link as LinkIcon,
  Monitor,
  Panda,
  Pencil,
  Plus,
  Settings2,
  Trash2,
} from "lucide-react";
import { useState } from "react";
import { Link as RouterLink, useNavigate, useParams } from "react-router";

import { PageBanner } from "@/components/page-banner";
import { useAuth } from "@/features/auth/auth-provider";
import { ApiError } from "@/lib/api/errors";

import type { Library, LibraryInput } from "./api";
import { changeLibrary, getLibrary, listLibraries, setEnabled } from "./api";
import { DirectoryPicker } from "./directory-picker";
import { LibraryDetailView } from "./library-detail-view";
import { formatSTRMExample } from "./strm-format-example";

const typeLabels: Record<LibraryInput["media_type"], string> = {
  movie: "电影",
  tv: "电视剧",
  anime: "动漫",
  documentary: "纪录片",
  mixed: "混合",
};
const initial: LibraryInput = {
  name: "",
  directory_id: "",
  directory_path: "",
  media_type: "movie",
  output_path: "",
  generate_strm: true,
  strm_existing_file_policy: "skip",
  generate_nfo: true,
  generate_poster_cover: true,
  language_preference: "zh-CN",
  scrape_source: "tmdb",
  exclude_directory_path: "",
  include_subdirectories: true,
  follow_directory_changes: true,
  file_encoding: "UTF-8",
  link_type: "proxy",
  strm_direct_domain: "",
  strm_link_format: "original_name",
};
const formRowClass =
  "w-full gap-2 lg:grid lg:grid-cols-[7.5rem_minmax(0,1fr)] lg:items-center";
function Link({
  to,
  children,
  className,
  "aria-label": ariaLabel,
}: {
  to: string;
  children: React.ReactNode;
  className?: string;
  "aria-label"?: string;
}) {
  return (
    <HeroLink
      href={to}
      {...(className ? { className } : {})}
      {...(ariaLabel ? { "aria-label": ariaLabel } : {})}
      render={(props) =>
        "href" in props ? (
          <RouterLink {...props} to={to} />
        ) : (
          <span {...props} />
        )
      }
    >
      {children}
    </HeroLink>
  );
}
function Banner({
  title,
  description,
  back = false,
}: {
  title: string;
  description: string;
  back?: boolean;
}) {
  return (
    <PageBanner
      title={
        <>
          {back && (
            <RouterLink
              to="/libraries"
              aria-label="返回媒体库"
              className="mr-3 inline-flex align-middle text-foreground"
            >
              <ArrowLeft />
            </RouterLink>
          )}
          {title}
        </>
      }
      description={description}
    />
  );
}
function ErrorMessage({ error }: { error: unknown }) {
  return error ? (
    <Alert status="danger" role="alert">
      <Alert.Indicator />
      <Alert.Content>
        <Alert.Description>
          {error instanceof Error ? error.message : "操作失败，请重试。"}
        </Alert.Description>
      </Alert.Content>
    </Alert>
  ) : null;
}
function Status({ enabled }: { enabled: boolean }) {
  return (
    <Chip size="sm" variant="soft" color={enabled ? "success" : "warning"}>
      <span
        aria-hidden="true"
        className="inline-block size-2 rounded-full bg-current"
      />
      {enabled ? "已启用" : "已暂停"}
    </Chip>
  );
}
function LibraryIcon({
  type,
  large = false,
}: {
  type: LibraryInput["media_type"];
  large?: boolean;
}) {
  const Icon =
    type === "tv"
      ? Monitor
      : type === "documentary"
        ? FileText
        : type === "anime"
          ? Panda
          : type === "mixed"
            ? Layers
            : Film;
  return (
    <span className="inline-flex size-12 shrink-0 items-center justify-center">
      <Icon aria-hidden="true" className={large ? "size-8" : "size-6"} />
    </span>
  );
}
function useActions() {
  const auth = useAuth();
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({
      action,
      library,
    }: {
      action: "delete" | "toggle";
      library: Library;
    }) =>
      action === "delete"
        ? changeLibrary("DELETE", auth.data?.csrf_token ?? "", library.id)
        : setEnabled(auth.data?.csrf_token ?? "", library.id, !library.enabled),
    onSuccess: () => client.invalidateQueries({ queryKey: ["libraries"] }),
  });
}
export function LibraryListPage() {
  const navigate = useNavigate();
  const [filter, setFilter] = useState<"all" | "enabled" | "paused">("all");
  const [deleting, setDeleting] = useState<Library | null>(null);
  const query = useQuery({
    queryKey: ["libraries", "list"],
    queryFn: ({ signal }) => listLibraries(signal),
    retry: false,
  });
  const actions = useActions();
  const deleteAction = useActions();
  const items =
    query.data?.filter(
      (item) => filter === "all" || item.enabled === (filter === "enabled"),
    ) ?? [];
  return (
    <main className="mx-auto w-full max-w-[1600px] px-[14px] py-[14px] min-[701px]:px-6 min-[701px]:py-6 min-[701px]:pb-12">
      <Banner
        title="媒体库"
        description="管理基于 115 网盘的媒体库，配置目录和输出规则。"
      />
      <div className="my-4 flex flex-col items-start justify-between gap-3 md:flex-row md:items-center">
        <ToggleButtonGroup
          className="flex flex-wrap"
          aria-label="筛选媒体库"
          selectionMode="single"
          disallowEmptySelection
          selectedKeys={new Set([filter])}
          onSelectionChange={(keys) => {
            const key = [...keys][0];
            if (key === "all" || key === "enabled" || key === "paused")
              setFilter(key);
          }}
        >
          {(
            [
              ["all", "全部"],
              ["enabled", "已启用"],
              ["paused", "已暂停"],
            ] as const
          ).map(([key, label]) => (
            <ToggleButton id={key} key={key}>
              {label}{" "}
              <span>
                {query.data
                  ? key === "all"
                    ? query.data.length
                    : query.data.filter(
                        (item) => item.enabled === (key === "enabled"),
                      ).length
                  : "—"}
              </span>
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
        <Link to="/libraries/new">
          <Plus /> 新建媒体库
        </Link>
      </div>
      <ErrorMessage error={actions.error} />
      {query.isPending ? (
        <p role="status">正在加载媒体库…</p>
      ) : query.isError ? (
        <div className="flex flex-col items-center gap-4 py-14 text-center">
          <p>无法加载媒体库。</p>
          <Button variant="ghost" onPress={() => void query.refetch()}>
            重试
          </Button>
        </div>
      ) : items.length === 0 ? (
        <div className="flex flex-col items-center gap-4 py-14 text-center">
          <Film />
          <h2>暂无媒体库</h2>
          <p>
            {filter === "all"
              ? "创建媒体库，开始管理您的媒体配置。"
              : "当前筛选条件下没有媒体库。"}
          </p>
          {filter === "all" && <Link to="/libraries/new">新建媒体库</Link>}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {items.map((item) => (
            <Card className="min-w-0" key={item.id}>
              <div className="flex min-w-0 items-start gap-4">
                <div className="flex size-16 shrink-0 items-center justify-center rounded-2xl bg-accent/10 text-accent">
                  <LibraryIcon type={item.media_type} large />
                </div>
                <div className="min-w-0 flex-1">
                  <Link
                    className="wrap-anywhere font-semibold"
                    to={`/libraries/${item.id}`}
                  >
                    {item.name}
                  </Link>
                  <div className="mt-1 flex flex-wrap items-center gap-2">
                    <Chip size="sm" variant="soft" color="accent">
                      {typeLabels[item.media_type]}
                    </Chip>
                    <Status enabled={item.enabled} />
                  </div>
                  <div className="mt-4 grid min-w-0 grid-cols-1 gap-x-3 gap-y-2 wrap-anywhere text-sm min-[1200px]:grid-cols-2">
                    <p className="m-0">
                      <span className="text-muted">绑定目录</span>{" "}
                      {item.directory_path}
                    </p>
                    <p className="m-0 min-[1200px]:border-l min-[1200px]:border-border min-[1200px]:pl-3">
                      <span className="text-muted">输出目录</span>{" "}
                      {item.output_path}
                    </p>
                    <p className="m-0">
                      <span className="text-muted">STRM 数量</span> 暂无数据
                    </p>
                    <p className="m-0 min-[1200px]:border-l min-[1200px]:border-border min-[1200px]:pl-3">
                      <span className="text-muted">最近同步</span> 暂无数据
                    </p>
                  </div>
                </div>
              </div>
              <footer className="mt-auto flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-2">
                  <Button
                    variant="secondary"
                    size="sm"
                    onPress={() => void navigate(`/libraries/${item.id}/edit`)}
                  >
                    <Pencil /> 编辑
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    isDisabled={actions.isPending || deleteAction.isPending}
                    onPress={() => {
                      deleteAction.reset();
                      setDeleting(item);
                    }}
                  >
                    <Trash2 /> 删除
                  </Button>
                </div>
                <Switch
                  className="ml-auto"
                  aria-label={`${item.name}启用状态`}
                  isSelected={item.enabled}
                  isDisabled={actions.isPending}
                  onChange={() =>
                    actions.mutate({ action: "toggle", library: item })
                  }
                >
                  <Switch.Content>
                    {item.enabled ? "已启用" : "已暂停"}
                    <Switch.Control>
                      <Switch.Thumb />
                    </Switch.Control>
                  </Switch.Content>
                </Switch>
              </footer>
            </Card>
          ))}
        </div>
      )}
      {deleting && (
        <AlertDialog.Backdrop
          isOpen
          isDismissable={false}
          isKeyboardDismissDisabled={deleteAction.isPending}
          onOpenChange={(open) => {
            if (!open && !deleteAction.isPending) setDeleting(null);
          }}
        >
          <AlertDialog.Container size="sm" placement="center">
            <AlertDialog.Dialog aria-describedby="delete-description">
              <AlertDialog.Header>
                <AlertDialog.Icon
                  status="danger"
                  className="rounded-full"
                  aria-hidden="true"
                >
                  <Trash2 className="size-5" />
                </AlertDialog.Icon>
                <AlertDialog.Heading className="text-lg font-semibold">
                  删除媒体库？
                </AlertDialog.Heading>
              </AlertDialog.Header>
              <AlertDialog.Body>
                <div
                  id="delete-description"
                  className="space-y-2 wrap-anywhere"
                >
                  <p>
                    确定删除“
                    <span className="font-medium text-foreground">
                      {deleting.name}
                    </span>
                    ”的配置？
                  </p>
                  <p>仅删除媒体库配置，网盘及本地文件不会被删除。</p>
                </div>
                {deleteAction.isPending && (
                  <p role="status" className="mt-3">
                    正在删除配置，请稍候…
                  </p>
                )}
                {deleteAction.error && (
                  <div className="mt-3">
                    <ErrorMessage error={deleteAction.error} />
                  </div>
                )}
              </AlertDialog.Body>
              <AlertDialog.Footer>
                <Button
                  variant="secondary"
                  // eslint-disable-next-line jsx-a11y/no-autofocus -- Focus the safe action when this alert dialog opens.
                  autoFocus
                  isDisabled={deleteAction.isPending}
                  onPress={() => {
                    if (!deleteAction.isPending) setDeleting(null);
                  }}
                >
                  取消
                </Button>
                <Button
                  variant="danger"
                  isDisabled={deleteAction.isPending}
                  onPress={() => {
                    if (deleteAction.isPending) return;
                    deleteAction.mutate(
                      { action: "delete", library: deleting },
                      { onSuccess: () => setDeleting(null) },
                    );
                  }}
                >
                  {deleteAction.isPending ? "正在删除…" : "确认删除"}
                </Button>
              </AlertDialog.Footer>
            </AlertDialog.Dialog>
          </AlertDialog.Container>
        </AlertDialog.Backdrop>
      )}
    </main>
  );
}
function Section({
  title,
  icon: Icon,
  children,
}: {
  title: string;
  icon: typeof Film;
  children: React.ReactNode;
}) {
  return (
    <Card className="min-w-0">
      <Card.Header className="flex-row items-center gap-2">
        <Icon aria-hidden="true" className="size-5 text-accent" />
        <Card.Title>{title}</Card.Title>
      </Card.Header>
      <Card.Content className="flex flex-col gap-3">{children}</Card.Content>
    </Card>
  );
}
function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <TextField className={formRowClass}>
      <Label>{label}</Label>
      <div className="min-w-0">{children}</div>
    </TextField>
  );
}
function LibrarySelect({
  label,
  value,
  options,
  onChange,
  disabled = false,
}: {
  label: string;
  value: string;
  options: [string, string][];
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <Select
      className={formRowClass}
      selectedKey={value}
      isDisabled={disabled}
      onSelectionChange={(key) => {
        if (typeof key === "string") onChange(key);
      }}
    >
      <Label>{label}</Label>
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {options.map(([key, text]) => (
            <ListBox.Item id={key} key={key} textValue={text}>
              {text}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  );
}
function OutputOption({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <Checkbox isSelected={checked} onChange={onChange}>
      <Checkbox.Content>
        <Checkbox.Control>
          <Checkbox.Indicator />
        </Checkbox.Control>
        {label}
      </Checkbox.Content>
    </Checkbox>
  );
}
function Toggle({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <Switch isSelected={checked} onChange={onChange}>
      <Switch.Content className="flex w-full items-center justify-between gap-3">
        {label}
        <Switch.Control>
          <Switch.Thumb />
        </Switch.Control>
      </Switch.Content>
    </Switch>
  );
}
export function LibraryFormPage() {
  const { id } = useParams();
  const query = useQuery({
    queryKey: ["libraries", "detail", id],
    queryFn: ({ signal }) => getLibrary(id ?? "", signal),
    enabled: !!id,
    retry: false,
  });
  if (id && query.isPending)
    return (
      <main className="mx-auto w-full max-w-[1600px] px-[14px] py-[14px] min-[701px]:px-6 min-[701px]:py-6 min-[701px]:pb-12">
        正在加载配置…
      </main>
    );
  if (id && !query.data)
    return (
      <main className="mx-auto w-full max-w-[1600px] px-[14px] py-[14px] min-[701px]:px-6 min-[701px]:py-6 min-[701px]:pb-12">
        <ErrorMessage error={query.error} />
        <Link to="/libraries">返回媒体库</Link>
      </main>
    );
  return (
    <LibraryForm
      key={id ?? "new"}
      id={id}
      initialValue={query.data ?? initial}
    />
  );
}
function LibraryForm({
  id,
  initialValue,
}: {
  id: string | undefined;
  initialValue: LibraryInput & { revision?: number };
}) {
  const navigate = useNavigate();
  const auth = useAuth();
  const client = useQueryClient();
  // Keep response metadata out of the editable draft and strict request body.
  const [value, setDraft] = useState<LibraryInput & { revision?: number }>(
    () => ({
      name: initialValue.name,
      directory_id: initialValue.directory_id,
      directory_path: initialValue.directory_path,
      media_type: initialValue.media_type,
      output_path: initialValue.output_path,
      generate_strm: initialValue.generate_strm,
      strm_existing_file_policy: initialValue.strm_existing_file_policy,
      generate_nfo: initialValue.generate_nfo,
      generate_poster_cover: initialValue.generate_poster_cover,
      language_preference: initialValue.language_preference,
      scrape_source: initialValue.scrape_source,
      exclude_directory_path: initialValue.exclude_directory_path,
      include_subdirectories: initialValue.include_subdirectories,
      follow_directory_changes: initialValue.follow_directory_changes,
      file_encoding: initialValue.file_encoding,
      link_type: initialValue.link_type,
      strm_direct_domain: initialValue.strm_direct_domain,
      strm_link_format: initialValue.strm_link_format,
      ...(initialValue.revision !== undefined
        ? { revision: initialValue.revision }
        : {}),
    }),
  );
  const [picker, setPicker] = useState(false);
  const [excludePicker, setExcludePicker] = useState(false);
  const [attempted, setAttempted] = useState(false);
  const update = (patch: Partial<LibraryInput>) =>
    setDraft((current) => ({ ...current, ...patch }));
  const mutation = useMutation({
    mutationFn: async (input: LibraryInput & { revision?: number }) => {
      const { revision, ...config } = input;
      return changeLibrary(
        id ? "PUT" : "POST",
        auth.data?.csrf_token ?? "",
        id,
        config,
        revision,
      );
    },
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["libraries"] });
      void navigate("/libraries");
    },
  });
  const directoryError = !value.directory_id
    ? "请选择绑定的 115 网盘目录。"
    : "";
  const nameError = !value.name.trim() ? "请输入媒体库名称。" : "";
  const outputError = !value.output_path.startsWith("/")
    ? "请输入以 / 开头的本地绝对路径。"
    : value.output_path.length > 1024
      ? "输出目录不能超过 1024 个字符。"
      : "";
  return (
    <main className="mx-auto w-full max-w-[1600px] px-[14px] py-[14px] min-[701px]:px-6 min-[701px]:py-6 min-[701px]:pb-12">
      <Form
        validationBehavior="aria"
        onSubmit={(event) => {
          event.preventDefault();
          setAttempted(true);
          if (nameError || outputError || directoryError) return;
          mutation.mutate(value);
        }}
      >
        <header className="flex flex-col gap-4 rounded-2xl bg-surface-secondary p-4 text-foreground min-[701px]:min-h-20 min-[701px]:flex-row min-[701px]:items-center min-[701px]:justify-between min-[701px]:px-7 min-[701px]:py-4">
          <div className="flex min-w-0 items-center gap-4">
            <Folder
              aria-hidden="true"
              className="size-8 shrink-0 text-accent min-[701px]:size-9"
            />
            <div className="min-w-0">
              <h1 className="m-0 text-xl font-semibold tracking-tight min-[701px]:text-2xl">
                {id ? "编辑媒体库配置" : "新建媒体库配置"}
              </h1>
              <p className="mt-1 mb-0 text-sm text-muted">
                配置媒体库基本信息、目录绑定、输出与 STRM 链接，保存后生效。
              </p>
            </div>
          </div>
          <div className="flex shrink-0 items-center justify-end gap-2">
            <Button
              variant="secondary"
              type="button"
              onPress={() => void navigate("/libraries")}
            >
              取消
            </Button>
            <Button
              variant="primary"
              type="submit"
              isDisabled={mutation.isPending}
            >
              {mutation.isPending ? "保存中…" : id ? "保存配置" : "创建媒体库"}
            </Button>
          </div>
        </header>
        <div className="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-2">
          <Section title="基本信息" icon={Layers}>
            <TextField
              className={formRowClass}
              isInvalid={attempted && !!nameError}
            >
              <Label>媒体库名称 *</Label>
              <div className="min-w-0">
                <Input
                  required
                  maxLength={100}
                  value={value.name}
                  onChange={(event) => update({ name: event.target.value })}
                  placeholder="例如：电影库"
                />
                <FieldError>{nameError}</FieldError>
              </div>
            </TextField>
            <LibrarySelect
              label="媒体类型"
              value={value.media_type}
              options={Object.entries(typeLabels)}
              onChange={(key) => {
                if (key in typeLabels)
                  update({ media_type: key as LibraryInput["media_type"] });
              }}
            />
            <LibrarySelect
              label="语言偏好"
              value={value.language_preference}
              options={[["zh-CN", "简体中文"]]}
              onChange={() => update({ language_preference: "zh-CN" })}
            />
            <LibrarySelect
              label="刮削源"
              value={value.scrape_source}
              options={[
                ["tmdb", "TMDB（推荐）"],
                ["metatube", "MetaTube"],
                ["none", "不刮削"],
              ]}
              onChange={(key) => {
                if (["tmdb", "metatube", "none"].includes(key))
                  update({
                    scrape_source: key as LibraryInput["scrape_source"],
                  });
              }}
            />
          </Section>
          <Section title="目录绑定" icon={Folder}>
            <TextField
              className={formRowClass}
              isInvalid={attempted && !!directoryError}
            >
              <Label id="library-directory-label">媒体库目录 *</Label>
              <div className="min-w-0">
                <Button
                  aria-labelledby="library-directory-label"
                  {...(attempted && directoryError
                    ? { "aria-describedby": "library-directory-error" }
                    : {})}
                  variant="outline"
                  className="w-full justify-between"
                  type="button"
                  onPress={() => setPicker(true)}
                >
                  <span className="truncate">
                    {value.directory_path || "请选择网盘目录"}
                  </span>
                  <Folder aria-hidden="true" />
                </Button>
                <FieldError id="library-directory-error">
                  {directoryError}
                </FieldError>
              </div>
            </TextField>
            <TextField className={formRowClass}>
              <Label>排除目录</Label>
              <div className="flex min-w-0 gap-2">
                <Input
                  className="min-w-0 flex-1"
                  value={value.exclude_directory_path}
                  onChange={(event) =>
                    update({ exclude_directory_path: event.target.value })
                  }
                  placeholder="可选，输入网盘目录路径"
                  maxLength={1024}
                />
                <Button
                  type="button"
                  variant="outline"
                  aria-label="选择排除目录"
                  onPress={() => setExcludePicker(true)}
                >
                  <Folder aria-hidden="true" />
                </Button>
              </div>
            </TextField>
            <Toggle
              label="包含子目录（递归扫描）"
              checked={value.include_subdirectories}
              onChange={(checked) =>
                update({ include_subdirectories: checked })
              }
            />
            <Toggle
              label="跟随网盘目录变更"
              checked={value.follow_directory_changes}
              onChange={(checked) =>
                update({ follow_directory_changes: checked })
              }
            />
          </Section>
          <Section title="输出设置" icon={Settings2}>
            <TextField
              className={formRowClass}
              isInvalid={attempted && !!outputError}
            >
              <Label>输出目录 *</Label>
              <div className="min-w-0">
                <InputGroup>
                  <InputGroup.Input
                    required
                    value={value.output_path}
                    onChange={(event) =>
                      update({ output_path: event.target.value })
                    }
                    placeholder="/app/media/电影"
                    maxLength={1024}
                  />
                  <InputGroup.Suffix aria-hidden="true">
                    <Folder className="size-4 text-muted" />
                  </InputGroup.Suffix>
                </InputGroup>
                <FieldError>{outputError}</FieldError>
              </div>
            </TextField>
            <div className="grid min-w-0 gap-2 lg:grid-cols-[6rem_minmax(0,1fr)] lg:items-center">
              <Label id="library-output-options-label">生成内容</Label>
              <div
                role="group"
                aria-labelledby="library-output-options-label"
                className="grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-3"
              >
                <OutputOption
                  label="STRM 文件"
                  checked={value.generate_strm}
                  onChange={(checked) => update({ generate_strm: checked })}
                />
                <OutputOption
                  label="NFO 信息"
                  checked={value.generate_nfo}
                  onChange={(checked) => update({ generate_nfo: checked })}
                />
                <OutputOption
                  label="海报与封面"
                  checked={value.generate_poster_cover}
                  onChange={(checked) =>
                    update({ generate_poster_cover: checked })
                  }
                />
              </div>
            </div>
            <LibrarySelect
              label="STRM 已存在时"
              value={value.strm_existing_file_policy}
              disabled={!value.generate_strm}
              options={[
                ["skip", "跳过（默认）"],
                ["overwrite", "覆盖"],
              ]}
              onChange={(key) => {
                if (key === "skip" || key === "overwrite")
                  update({ strm_existing_file_policy: key });
              }}
            />
            <LibrarySelect
              label="文件编码"
              value={value.file_encoding}
              options={[["UTF-8", "UTF-8（推荐）"]]}
              onChange={() => update({ file_encoding: "UTF-8" })}
            />
          </Section>
          <Section title="STRM 链接配置" icon={LinkIcon}>
            <LibrarySelect
              label="链接类型"
              value={value.link_type}
              options={[
                ["direct", "直连"],
                ["proxy", "302 代理"],
              ]}
              onChange={(key) =>
                update({ link_type: key === "direct" ? "direct" : "proxy" })
              }
            />
            <Field label="STRM 直连域名">
              <Input
                value={value.strm_direct_domain}
                onChange={(event) =>
                  update({ strm_direct_domain: event.target.value })
                }
                placeholder="例如：media.example.com"
                maxLength={253}
              />
            </Field>
            <div className="min-w-0">
              <Tabs
                className="grid min-w-0 gap-x-2 gap-y-3 lg:grid-cols-[7.5rem_minmax(0,1fr)] lg:items-center"
                aria-labelledby="strm-link-format-label"
                selectedKey={value.strm_link_format}
                onSelectionChange={(key) => {
                  if (key === "original_name" || key === "file_id")
                    update({ strm_link_format: key });
                }}
              >
                <Label id="strm-link-format-label">链接格式</Label>
                <Tabs.ListContainer>
                  <Tabs.List aria-label="STRM 链接格式">
                    <Tabs.Tab
                      className="data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground"
                      id="original_name"
                    >
                      文件原名
                    </Tabs.Tab>
                    <Tabs.Tab
                      className="data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground"
                      id="file_id"
                    >
                      文件 ID
                    </Tabs.Tab>
                  </Tabs.List>
                </Tabs.ListContainer>
                <Tabs.Panel
                  id="original_name"
                  className="min-w-0 lg:col-start-2"
                >
                  <p className="wrap-anywhere text-sm text-muted">
                    格式示例：
                    {formatSTRMExample(
                      value.strm_direct_domain,
                      "original_name",
                    )}
                  </p>
                </Tabs.Panel>
                <Tabs.Panel id="file_id" className="min-w-0 lg:col-start-2">
                  <p className="wrap-anywhere text-sm text-muted">
                    格式示例：
                    {formatSTRMExample(value.strm_direct_domain, "file_id")}
                  </p>
                </Tabs.Panel>
              </Tabs>
            </div>
          </Section>
        </div>
        <ErrorMessage error={mutation.error} />
        {mutation.error instanceof ApiError && mutation.error.fields && (
          <Alert status="danger" role="alert">
            <Alert.Indicator />
            <Alert.Content>
              <Alert.Description>
                <ul>
                  {Object.entries(mutation.error.fields).map(
                    ([field, message]) => (
                      <li key={field}>
                        {field}：{message}
                      </li>
                    ),
                  )}
                </ul>
              </Alert.Description>
            </Alert.Content>
          </Alert>
        )}
      </Form>
      {picker && (
        <DirectoryPicker
          title="选择媒体库目录"
          onClose={() => setPicker(false)}
          onSelect={(directory_id, directory_path) => {
            update({ directory_id, directory_path });
            setPicker(false);
          }}
        />
      )}
      {excludePicker && (
        <DirectoryPicker
          title="选择排除目录"
          onClose={() => setExcludePicker(false)}
          onSelect={(_, directory_path) => {
            update({ exclude_directory_path: directory_path });
            setExcludePicker(false);
          }}
        />
      )}
    </main>
  );
}
export function LibraryDetailPage() {
  const { id } = useParams();
  const query = useQuery({
    queryKey: ["libraries", "detail", id],
    queryFn: ({ signal }) => getLibrary(id ?? "", signal),
    enabled: !!id,
    retry: false,
  });
  const actions = useActions();
  if (query.isPending)
    return (
      <main className="mx-auto w-full max-w-[1600px] px-[14px] py-[14px] min-[701px]:px-6 min-[701px]:py-6 min-[701px]:pb-12">
        正在加载媒体库…
      </main>
    );
  if (!query.data)
    return (
      <main className="mx-auto w-full max-w-[1600px] px-[14px] py-[14px] min-[701px]:px-6 min-[701px]:py-6 min-[701px]:pb-12">
        <ErrorMessage error={query.error} />
        <Link to="/libraries">返回媒体库</Link>
      </main>
    );
  const item = query.data;
  return (
    <main className="mx-auto w-full max-w-[1600px] px-[14px] py-[14px] min-[701px]:px-6 min-[701px]:py-6 min-[701px]:pb-12">
      <ErrorMessage error={actions.error} />
      <LibraryDetailView
        item={item}
        pending={actions.isPending}
        onToggle={() => actions.mutate({ action: "toggle", library: item })}
      />
    </main>
  );
}
