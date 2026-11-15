import {
  Alert,
  Button,
  Card,
  Chip,
  FieldError,
  Form,
  Input,
  Label,
  Link as HeroLink,
  ListBox,
  Modal,
  Radio,
  RadioGroup,
  Select,
  Switch,
  TextArea,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
} from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  CheckCircle2,
  Clapperboard,
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
  WandSparkles,
} from "lucide-react";
import { useState } from "react";
import { Link as RouterLink, useNavigate, useParams } from "react-router";

import { PageBanner } from "@/components/page-banner";
import { useAuth } from "@/features/auth/auth-provider";
import { ApiError } from "@/lib/api/errors";

import type { Library, LibraryInput } from "./api";
import { changeLibrary, getLibrary, listLibraries, setEnabled } from "./api";
import { DirectoryPicker } from "./directory-picker";

const typeLabels: Record<LibraryInput["media_type"], string> = {
  movie: "电影",
  tv: "电视剧",
  anime: "动漫",
  documentary: "纪录片",
  mixed: "混合",
};
const initial: LibraryInput = {
  name: "",
  description: "",
  directory_id: "",
  directory_path: "",
  media_type: "movie",
  enabled: true,
  output_path: "",
  generate_strm: true,
  scrape_metadata: true,
  generate_nfo: true,
  preserve_structure: true,
  structure_mode: "full",
  path_mappings: [],
  link_type: "direct",
  link_expiry: "default",
  filename_rule: "original",
  strm_extension: ".strm",
  nfo_name: "same_as_video",
  min_file_size_mb: 100,
};
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
            <Link
              to="/libraries"
              aria-label="返回媒体库"
              className="mr-3 inline-flex align-middle text-foreground"
            >
              <ArrowLeft />
            </Link>
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
                    onPress={() => setDeleting(item)}
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
        <Modal.Backdrop
          isOpen
          onOpenChange={(open) => {
            if (!open) setDeleting(null);
          }}
        >
          <Modal.Container>
            <Modal.Dialog aria-labelledby="delete-title">
              <h2 id="delete-title">删除媒体库？</h2>
              <p>确定删除“{deleting.name}”的配置？网盘及本地文件不会被删除。</p>
              <ErrorMessage error={actions.error} />
              <footer>
                <Button variant="ghost" onPress={() => setDeleting(null)}>
                  取消
                </Button>
                <Button
                  variant="danger"
                  isDisabled={actions.isPending}
                  onPress={() =>
                    actions.mutate(
                      { action: "delete", library: deleting },
                      { onSuccess: () => setDeleting(null) },
                    )
                  }
                >
                  确认删除
                </Button>
              </footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
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
    <Card className="min-w-0 last:md:col-span-2">
      <h2 className="flex items-center gap-2 font-semibold">
        <Icon className="size-5" />
        {title}
      </h2>
      {children}
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
    <TextField className="my-2 w-full">
      <Label>{label}</Label>
      {children}
    </TextField>
  );
}
function LibrarySelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: [string, string][];
  onChange: (value: string) => void;
}) {
  return (
    <Select
      className="my-2 w-full"
      selectedKey={value}
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
      <Switch.Content>
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
  const [value, setDraft] = useState(initialValue);
  const [picker, setPicker] = useState(false);
  const [attempted, setAttempted] = useState(false);
  const update = (patch: Partial<LibraryInput>) =>
    setDraft((current) => ({ ...current, ...patch }));
  const mutation = useMutation({
    mutationFn: async (input: LibraryInput & { revision?: number }) => {
      const {
        name,
        description,
        directory_id,
        directory_path,
        media_type,
        enabled,
        output_path,
        generate_strm,
        scrape_metadata,
        generate_nfo,
        preserve_structure,
        structure_mode,
        path_mappings,
        link_type,
        link_expiry,
        filename_rule,
        strm_extension,
        nfo_name,
        min_file_size_mb,
      } = input;
      return changeLibrary(
        id ? "PUT" : "POST",
        auth.data?.csrf_token ?? "",
        id,
        {
          name,
          description,
          directory_id,
          directory_path,
          media_type,
          enabled,
          output_path,
          generate_strm,
          scrape_metadata,
          generate_nfo,
          preserve_structure,
          structure_mode,
          path_mappings,
          link_type,
          link_expiry,
          filename_rule,
          strm_extension,
          nfo_name,
          min_file_size_mb,
        },
        input.revision,
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
  const mappingError = value.path_mappings.some(
    (mapping) =>
      !mapping.source.trim() ||
      !mapping.target.trim() ||
      mapping.source.length > 1024 ||
      mapping.target.length > 1024,
  )
    ? "每条路径映射的源路径和目标路径均不能为空，且不能超过 1024 个字符。"
    : value.path_mappings.length > 100
      ? "路径映射最多 100 条。"
      : "";
  const sizeError =
    !Number.isInteger(value.min_file_size_mb) ||
    value.min_file_size_mb < 0 ||
    value.min_file_size_mb > 1048576
      ? "请输入 0 到 1048576 之间的整数。"
      : "";
  return (
    <main className="mx-auto w-full max-w-[1600px] px-[14px] py-[14px] min-[701px]:px-6 min-[701px]:py-6 min-[701px]:pb-12">
      <Banner
        title={id ? "编辑媒体库" : "新建媒体库"}
        description="配置目录、输出、STRM 链接及文件规则。"
        back
      />
      <Form
        validationBehavior="aria"
        onSubmit={(event) => {
          event.preventDefault();
          setAttempted(true);
          if (
            nameError ||
            outputError ||
            mappingError ||
            sizeError ||
            directoryError
          )
            return;
          mutation.mutate(value);
        }}
      >
        <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
          <Section title="基本信息" icon={Layers}>
            <TextField
              className="my-2 w-full"
              isInvalid={attempted && !!nameError}
            >
              <Label>媒体库名称 *</Label>
              <Input
                required
                maxLength={100}
                value={value.name}
                onChange={(e) => update({ name: e.target.value })}
                placeholder="例如：电影库"
              />
              <FieldError>{nameError}</FieldError>
            </TextField>
            <Field label="描述（可选）">
              <TextArea
                maxLength={200}
                value={value.description}
                onChange={(e) => update({ description: e.target.value })}
                placeholder="用于说明该媒体库的用途"
              />
            </Field>
            <small>{value.description.length} / 200</small>
          </Section>
          <Section title="绑定 115 网盘目录" icon={Folder}>
            <TextField
              className="my-2 w-full"
              isInvalid={attempted && !!directoryError}
            >
              <Label id="library-directory-label">选择绑定目录 *</Label>
              <Button
                aria-labelledby="library-directory-label"
                render={(props) => (
                  <button
                    {...props}
                    aria-describedby={
                      [
                        value.directory_path ? "library-directory-value" : "",
                        attempted && directoryError
                          ? "library-directory-error"
                          : "",
                      ]
                        .filter(Boolean)
                        .join(" ") || undefined
                    }
                  />
                )}
                variant="ghost"
                className="w-full justify-between"
                type="button"
                onPress={() => setPicker(true)}
              >
                <Folder />{" "}
                <span id="library-directory-value">
                  {value.directory_path || "请选择网盘目录"}
                </span>{" "}
                <span>›</span>
              </Button>
              <FieldError id="library-directory-error">
                {directoryError}
              </FieldError>
            </TextField>
            <small>目录路径仅供展示，目录是否存在以网盘实际情况为准。</small>
          </Section>
          <Section title="媒体类型" icon={Film}>
            <RadioGroup
              aria-label="媒体类型"
              variant="primary"
              orientation="horizontal"
              value={value.media_type}
              onChange={(type) => {
                if (type in typeLabels)
                  update({ media_type: type as LibraryInput["media_type"] });
              }}
            >
              {(Object.keys(typeLabels) as LibraryInput["media_type"][]).map(
                (type) => (
                  <Radio key={type} value={type}>
                    <Radio.Content>
                      <Radio.Control>
                        <Radio.Indicator />
                      </Radio.Control>
                      <LibraryIcon type={type} />
                      {typeLabels[type]}
                    </Radio.Content>
                  </Radio>
                ),
              )}
            </RadioGroup>
          </Section>
          <Section title="输出设置" icon={Settings2}>
            <Toggle
              label="生成 STRM 文件"
              checked={value.generate_strm}
              onChange={(v) => update({ generate_strm: v })}
            />
            <Toggle
              label="刮削媒体信息"
              checked={value.scrape_metadata}
              onChange={(v) => update({ scrape_metadata: v })}
            />
            <Toggle
              label="同步生成 NFO 文件"
              checked={value.generate_nfo}
              onChange={(v) => update({ generate_nfo: v })}
            />
          </Section>
          <Section title="本地输出目录" icon={Folder}>
            <TextField
              className="my-2 w-full"
              isInvalid={attempted && !!outputError}
            >
              <Label>输出目录（本地） *</Label>
              <Input
                required
                value={value.output_path}
                onChange={(e) => update({ output_path: e.target.value })}
                placeholder="/app/media/电影"
              />
              <FieldError>{outputError}</FieldError>
            </TextField>
            <small>填写容器内的绝对路径，用于未来生成的文件。</small>
          </Section>
          <Section title="目录结构保留" icon={Layers}>
            <Toggle
              label="保留源目录结构"
              checked={value.preserve_structure}
              onChange={(v) => update({ preserve_structure: v })}
            />
            <LibrarySelect
              label="结构策略"
              value={value.structure_mode}
              options={[
                ["full", "完整保留"],
                ["flatten", "平铺"],
              ]}
              onChange={(key) =>
                update({
                  structure_mode: key === "flatten" ? "flatten" : "full",
                })
              }
            />
          </Section>
          <Section title="路径映射" icon={LinkIcon}>
            {value.path_mappings.map((mapping, index) => (
              <div className="my-2 flex items-start gap-2" key={index}>
                <TextField
                  aria-label={`映射 ${index + 1} 源路径`}
                  className="min-w-0 flex-1"
                  isInvalid={
                    attempted &&
                    (!mapping.source.trim() || mapping.source.length > 1024)
                  }
                >
                  <Input
                    required
                    placeholder="网盘路径"
                    value={mapping.source}
                    onChange={(e) =>
                      update({
                        path_mappings: value.path_mappings.map((item, i) =>
                          i === index
                            ? { ...item, source: e.target.value }
                            : item,
                        ),
                      })
                    }
                  />
                  <FieldError>请输入不超过 1024 个字符的源路径。</FieldError>
                </TextField>
                <span>→</span>
                <TextField
                  aria-label={`映射 ${index + 1} 目标路径`}
                  className="min-w-0 flex-1"
                  isInvalid={
                    attempted &&
                    (!mapping.target.trim() || mapping.target.length > 1024)
                  }
                >
                  <Input
                    required
                    placeholder="本地路径"
                    value={mapping.target}
                    onChange={(e) =>
                      update({
                        path_mappings: value.path_mappings.map((item, i) =>
                          i === index
                            ? { ...item, target: e.target.value }
                            : item,
                        ),
                      })
                    }
                  />
                  <FieldError>请输入不超过 1024 个字符的目标路径。</FieldError>
                </TextField>
                <Button
                  variant="ghost"
                  type="button"
                  aria-label={`删除映射 ${index + 1}`}
                  onPress={() =>
                    update({
                      path_mappings: value.path_mappings.filter(
                        (_, i) => i !== index,
                      ),
                    })
                  }
                >
                  <Trash2 />
                </Button>
              </div>
            ))}
            {attempted && value.path_mappings.length > 100 && (
              <Alert status="danger" role="alert">
                <Alert.Indicator />
                <Alert.Content>
                  <Alert.Description>路径映射最多 100 条。</Alert.Description>
                </Alert.Content>
              </Alert>
            )}
            <Button
              variant="ghost"

              type="button"
              onPress={() =>
                update({
                  path_mappings: [
                    ...value.path_mappings,
                    { source: "", target: "" },
                  ],
                })
              }
            >
              ＋ 添加路径映射
            </Button>
          </Section>
          <Section title="STRM 链接配置" icon={LinkIcon}>
            <LibrarySelect
              label="链接类型"
              value={value.link_type}
              options={[
                ["direct", "115 网盘直链"],
                ["proxy", "代理链接"],
              ]}
              onChange={(key) =>
                update({ link_type: key === "proxy" ? "proxy" : "direct" })
              }
            />
            <LibrarySelect
              label="链接有效期"
              value={value.link_expiry}
              options={[
                ["default", "默认"],
                ["never", "永不过期"],
              ]}
              onChange={(key) =>
                update({ link_expiry: key === "never" ? "never" : "default" })
              }
            />
          </Section>
          <Section title="文件规则" icon={FileText}>
            <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
              <LibrarySelect
                label="文件名规则"
                value={value.filename_rule}
                options={[["original", "保持原文件名"]]}
                onChange={() => update({ filename_rule: "original" })}
              />
              <Field label="STRM 扩展名">
                <Input value={value.strm_extension} readOnly />
              </Field>
              <LibrarySelect
                label="NFO 命名"
                value={value.nfo_name}
                options={[["same_as_video", "与视频同名"]]}
                onChange={() => update({ nfo_name: "same_as_video" })}
              />
              <TextField
                className="my-2 w-full"
                isInvalid={attempted && !!sizeError}
              >
                <Label>过滤小文件（MB）</Label>
                <Input
                  type="number"
                  min={0}
                  max={1048576}
                  required
                  value={value.min_file_size_mb}
                  onChange={(e) =>
                    update({ min_file_size_mb: Number(e.target.value) })
                  }
                />
                <FieldError>{sizeError}</FieldError>
              </TextField>
            </div>
          </Section>
        </div>
        <p className="mt-2 text-sm text-muted">
          以上选项仅保存为配置，扫描、同步和文件生成将在后续版本开放。
        </p>
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
        <div className="mt-5 flex items-center justify-end gap-4">
          <Link to="/libraries">取消</Link>
          <Button
            variant="primary"
            type="submit"
            isDisabled={mutation.isPending}
          >
            {mutation.isPending ? "保存中…" : id ? "保存配置" : "创建媒体库"}
          </Button>
        </div>
      </Form>
      {picker && (
        <DirectoryPicker
          onClose={() => setPicker(false)}
          onSelect={(directory_id, directory_path) => {
            update({ directory_id, directory_path });
            setPicker(false);
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
      <Banner title={item.name} description="查看媒体库配置与状态。" back />
      <div className="mt-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Button variant="ghost" isDisabled>
          全量同步
        </Button>
        <Button variant="ghost" isDisabled>
          增量同步
        </Button>
        <Button variant="ghost" isDisabled>
          执行预览
        </Button>
        <Link to={`/libraries/${item.id}/edit`}>
          <Pencil /> 编辑配置
        </Link>
      </div>
      <p className="mt-2 text-sm text-muted">同步和执行功能尚未开放。</p>
      <ErrorMessage error={actions.error} />
      <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
        <Section title="库概览" icon={Film}>
          <div className="flex items-center gap-3">
            <LibraryIcon type={item.media_type} />
            <div>
              <strong>{item.name}</strong>
              <p>
                <Chip size="sm" variant="soft" color="accent">
                  {typeLabels[item.media_type]}
                </Chip>{" "}
                <Status enabled={item.enabled} />
              </p>
            </div>
          </div>
          <p className="wrap-anywhere">{item.description || "暂无描述"}</p>
        </Section>
        <Section title="当前状态" icon={WandSparkles}>
          <Status enabled={item.enabled} />
          <p>配置状态不代表同步运行状态。</p>
          <Button
            variant="ghost"
            isDisabled={actions.isPending}
            onPress={() => actions.mutate({ action: "toggle", library: item })}
          >
            {item.enabled ? "暂停媒体库" : "启用媒体库"}
          </Button>
        </Section>
        <Section title="绑定目录" icon={Folder}>
          <p className="break-all">{item.directory_path}</p>
          <small>目录是否存在以网盘实际情况为准。</small>
        </Section>
        <Section title="输出设置摘要" icon={Settings2}>
          <p className="wrap-anywhere">输出目录：{item.output_path}</p>
          <p>目录结构：{item.preserve_structure ? "保留" : "不保留"}</p>
          <p>生成 NFO：{item.generate_nfo ? "已启用" : "已关闭"}</p>
        </Section>
        <Section title="STRM 统计" icon={LinkIcon}>
          <p>暂无数据</p>
        </Section>
        <Section title="最近同步时间" icon={CheckCircle2}>
          <p>暂无数据</p>
        </Section>
        <Section title="执行记录摘要" icon={Clapperboard}>
          <p>暂无执行记录</p>
        </Section>
      </div>
    </main>
  );
}
