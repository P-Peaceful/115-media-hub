import {
  Button,
  Checkbox,
  CheckboxGroup,
  Chip,
  Description,
  Form,
  Input,
  Label,
  Radio,
  RadioGroup,
  Switch,
  TextField,
  TimeField,
} from "@heroui/react";
import { parseTime } from "@internationalized/date";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, CalendarDays, Film, Layers3, Save } from "lucide-react";
import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";

import type { Library } from "@/features/media-library/api";
import { ApiError } from "@/lib/api/errors";

import type { Schedule, TaskTypeCapability } from "./api";
import { getSchedule, saveSchedule } from "./api";
import {
  scheduleKeys,
  useTaskCapabilities,
  useTaskCsrf,
  useTaskLibraries,
} from "./queries";
import type { ScheduleInput, TaskOutcome, TaskType } from "./task-kinds";
import { hasKey, keys, outcomes, periods, taskTypes } from "./task-kinds";
import {
  defaultTimezone,
  headingClass,
  Panel,
  TaskBackLink,
  TaskButtonLink,
  TaskError,
  TaskLoading,
  taskPageClass,
  TaskSelect,
  titleClass,
  toolbarClass,
} from "./task-ui";

const weekdays = [
  { id: 1, label: "周一" },
  { id: 2, label: "周二" },
  { id: 3, label: "周三" },
  { id: 4, label: "周四" },
  { id: 5, label: "周五" },
  { id: 6, label: "周六" },
  { id: 0, label: "周日" },
];
const monthDays = Array.from({ length: 31 }, (_, index) => ({
  id: String(index + 1),
  label: `${index + 1} 日`,
}));
const allLibraries = "all";

function isTimeZone(timezone: string) {
  try {
    new Intl.DateTimeFormat("zh-CN", { timeZone: timezone });
    return true;
  } catch {
    return false;
  }
}

function ScheduleEditor({
  current,
  capabilities,
  libraries,
  backHref,
}: {
  current: Schedule | null;
  capabilities: TaskTypeCapability[];
  libraries: Library[];
  backHref: string;
}) {
  const [original] = useState(current);
  const [name, setName] = useState(current?.name ?? "");
  const [type, setType] = useState<TaskType | "">(current?.type ?? "");
  const [library, setLibrary] = useState(current?.library_id ?? allLibraries);
  const [enabled, setEnabled] = useState(current?.enabled ?? true);
  const [period, setPeriod] = useState<ScheduleInput["period"]>(
    current?.period ?? "daily",
  );
  const [timezone, setTimezone] = useState(
    current?.timezone ?? defaultTimezone,
  );
  const [time, setTime] = useState(current?.time ?? "03:00");
  const [selectedDays, setSelectedDays] = useState(current?.weekdays ?? [1]);
  const [month, setMonth] = useState(String(current?.month_day ?? 1));
  const [cron, setCron] = useState(current?.cron ?? "");
  const [notificationEnabled, setNotificationEnabled] = useState(
    Boolean(current?.notify_on.length),
  );
  const [notifyOn, setNotifyOn] = useState<TaskOutcome[]>(
    current?.notify_on.length ? current.notify_on : keys(outcomes),
  );
  const [validation, setValidation] = useState("");
  const csrf = useTaskCsrf();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: (input: ScheduleInput) => saveSchedule(csrf(), original, input),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: scheduleKeys.all });
      void navigate(backHref);
    },
  });

  const capability = capabilities.find((entry) => entry.type === type);
  const typeOptions = keys(taskTypes).map((id) => ({
    id,
    label: capabilities.some((entry) => entry.type === id && entry.available)
      ? taskTypes[id].label
      : `${taskTypes[id].label}（暂不可用）`,
  }));
  const enabledLibraries = libraries.filter((item) => item.enabled);
  const libraryOptions = [
    { id: allLibraries, label: "全部媒体库" },
    ...enabledLibraries.map((item) => ({ id: item.id, label: item.name })),
  ];
  const libraryAvailable =
    library === allLibraries ||
    enabledLibraries.some((item) => item.id === library);
  if (!libraryAvailable) {
    libraryOptions.push({
      id: library,
      label: `${libraries.find((item) => item.id === library)?.name ?? library}（暂不可用）`,
    });
  }

  function submit() {
    if (!type || !capability?.available || !libraryAvailable) return;
    if (!name.trim()) return setValidation("请输入计划名称。");
    if (!isTimeZone(timezone.trim()))
      return setValidation("请输入有效的 IANA 时区，例如 Asia/Shanghai。");
    if (period === "custom" && cron.trim().split(/\s+/).length !== 5)
      return setValidation("请输入五字段 Cron 表达式。");
    if (period !== "custom" && !/^([01]\d|2[0-3]):[0-5]\d$/.test(time))
      return setValidation("请输入有效的执行时间。");
    if (period === "weekly" && !selectedDays.length)
      return setValidation("请至少选择一个重复日期。");
    if (notificationEnabled && !notifyOn.length)
      return setValidation("请至少选择一种通知结果。");
    setValidation("");
    mutation.mutate({
      name: name.trim(),
      type,
      library_id: library === allLibraries ? null : library,
      enabled,
      period,
      timezone: timezone.trim(),
      notify_on: notificationEnabled ? notifyOn : [],
      ...(period === "custom" ? { cron: cron.trim() } : { time }),
      ...(period === "weekly" ? { weekdays: selectedDays } : {}),
      ...(period === "monthly" ? { month_day: Number(month) } : {}),
    });
  }

  return (
    <main className={taskPageClass}>
      <Form
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
        className="grid min-w-0 gap-4"
      >
        <header className={headingClass}>
          <TaskBackLink label="返回定时计划" href={backHref} />
          <div className="min-w-0 flex-[1_1_15rem]">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className={titleClass}>
                {current ? `编辑计划 · ${current.name}` : "创建计划"}
              </h1>
              {current && (
                <Chip
                  size="sm"
                  variant="soft"
                  color={enabled ? "success" : "default"}
                >
                  <Chip.Label>{enabled ? "已启用" : "已停用"}</Chip.Label>
                </Chip>
              )}
            </div>
          </div>
          <div className={toolbarClass}>
            <TaskButtonLink href={backHref}>取消</TaskButtonLink>
            <Button
              size="sm"
              type="submit"
              isPending={mutation.isPending}
              isDisabled={!capability?.available || !libraryAvailable}
            >
              <Save aria-hidden="true" />
              保存
            </Button>
          </div>
        </header>
        {validation && (
          <TaskError error={new ApiError("schedule_validation", validation)} />
        )}
        <TaskError error={mutation.error} />
        <div className="grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-2">
          <Panel title="执行类型" icon={Layers3}>
            <TextField
              variant="secondary"
              name="name"
              isRequired
              value={name}
              onChange={setName}
            >
              <Label>计划名称</Label>
              <Input maxLength={160} />
            </TextField>
            <TaskSelect
              showLabel
              label="执行类型"
              value={type}
              options={typeOptions}
              onChange={(value) => {
                if (hasKey(taskTypes, value)) setType(value);
              }}
              description={
                capability?.available === false
                  ? (capability.reason ?? "此类型暂不可用。")
                  : !type && capabilities.every((item) => !item.available)
                    ? "暂无可用的任务类型。选择类型可查看原因。"
                    : null
              }
            />
          </Panel>
          <Panel
            title="触发周期"
            icon={CalendarDays}
            className="lg:col-start-2 lg:row-span-2 lg:row-start-1"
          >
            <RadioGroup
              variant="secondary"
              value={period}
              onChange={(value) => {
                if (hasKey(periods, value)) setPeriod(value);
              }}
            >
              <Label>执行周期</Label>
              <div className="flex flex-wrap gap-x-4 gap-y-2">
                {keys(periods).map((value) => (
                  <Radio key={value} value={value}>
                    <Radio.Content>
                      <Radio.Control>
                        <Radio.Indicator />
                      </Radio.Control>
                      <Label>{periods[value]}</Label>
                    </Radio.Content>
                  </Radio>
                ))}
              </div>
            </RadioGroup>
            {period === "weekly" && (
              <CheckboxGroup
                name="weekdays"
                value={selectedDays.map(String)}
                onChange={(values) =>
                  setSelectedDays(
                    values.map(Number).sort((first, second) => first - second),
                  )
                }
              >
                <Label>重复于</Label>
                <div className="flex flex-wrap gap-x-4 gap-y-2">
                  {weekdays.map((day) => (
                    <Checkbox
                      key={day.id}
                      value={String(day.id)}
                      variant="secondary"
                    >
                      <Checkbox.Content>
                        <Checkbox.Control>
                          <Checkbox.Indicator />
                        </Checkbox.Control>
                        {day.label}
                      </Checkbox.Content>
                    </Checkbox>
                  ))}
                </div>
              </CheckboxGroup>
            )}
            {period === "monthly" && (
              <TaskSelect
                showLabel
                label="每月执行日期"
                value={month}
                options={monthDays}
                onChange={setMonth}
              />
            )}
            {period === "custom" ? (
              <TextField
                variant="secondary"
                name="cron"
                isRequired
                value={cron}
                onChange={setCron}
              >
                <Label>Cron 表达式</Label>
                <Input maxLength={128} placeholder="0 3 * * *" />
                <Description>格式为分钟、小时、日、月和星期。</Description>
              </TextField>
            ) : (
              <TimeField
                name="time"
                isRequired
                granularity="minute"
                hourCycle={24}
                value={time ? parseTime(time) : null}
                onChange={(value) =>
                  setTime(value ? value.toString().slice(0, 5) : "")
                }
              >
                <Label>执行时间</Label>
                <TimeField.Group variant="secondary">
                  <TimeField.Input>
                    {(segment) => <TimeField.Segment segment={segment} />}
                  </TimeField.Input>
                </TimeField.Group>
              </TimeField>
            )}
            <TextField
              variant="secondary"
              name="timezone"
              isRequired
              value={timezone}
              onChange={setTimezone}
            >
              <Label>调度时区</Label>
              <Input maxLength={128} />
            </TextField>
          </Panel>
          <Panel title="目标媒体库" icon={Film}>
            <TaskSelect
              showLabel
              label="目标媒体库"
              value={library}
              options={libraryOptions}
              onChange={setLibrary}
            />
            <Switch isSelected={enabled} onChange={setEnabled}>
              <Switch.Content>
                <Label>启用定时计划</Label>
                <Switch.Control>
                  <Switch.Thumb />
                </Switch.Control>
              </Switch.Content>
            </Switch>
          </Panel>
          <Panel title="通知选项" icon={Bell} className="lg:col-span-2">
            <Switch
              isSelected={notificationEnabled}
              onChange={setNotificationEnabled}
            >
              <Switch.Content>
                <Label>任务完成后发送站内通知</Label>
                <Switch.Control>
                  <Switch.Thumb />
                </Switch.Control>
              </Switch.Content>
            </Switch>
            {notificationEnabled && (
              <CheckboxGroup
                name="notify_on"
                value={notifyOn}
                onChange={(values) =>
                  setNotifyOn(values.filter((item) => hasKey(outcomes, item)))
                }
              >
                <Label>通知范围</Label>
                <div className="flex flex-wrap gap-x-4 gap-y-2">
                  {keys(outcomes).map((outcome) => (
                    <Checkbox key={outcome} value={outcome} variant="secondary">
                      <Checkbox.Content>
                        <Checkbox.Control>
                          <Checkbox.Indicator />
                        </Checkbox.Control>
                        {outcomes[outcome]}
                      </Checkbox.Content>
                    </Checkbox>
                  ))}
                </div>
              </CheckboxGroup>
            )}
          </Panel>
        </div>
      </Form>
    </main>
  );
}

export function ScheduleEditorPage() {
  const { id } = useParams();
  const [search] = useSearchParams();
  const from = search.get("from");
  const backHref = `/tasks/schedules${from?.startsWith("?") ? from : ""}`;
  const capabilities = useTaskCapabilities();
  const libraries = useTaskLibraries();
  const schedule = useQuery({
    queryKey: scheduleKeys.detail(id ?? ""),
    queryFn: ({ signal }) => getSchedule(id ?? "", signal),
    enabled: Boolean(id),
    retry: false,
  });
  if (capabilities.data && libraries.data && (!id || schedule.data)) {
    return (
      <ScheduleEditor
        key={id ?? "new"}
        current={schedule.data ?? null}
        capabilities={capabilities.data}
        libraries={libraries.data}
        backHref={backHref}
      />
    );
  }

  const error = capabilities.error || libraries.error || schedule.error;
  return (
    <main className={taskPageClass}>
      <header className={headingClass}>
        <TaskBackLink label="返回定时计划" href={backHref} />
        <h1 className={titleClass}>{id ? "编辑计划" : "创建计划"}</h1>
      </header>
      {error ? (
        <TaskError
          error={error}
          retry={() => {
            if (capabilities.isError) void capabilities.refetch();
            if (libraries.isError) void libraries.refetch();
            if (id && schedule.isError) void schedule.refetch();
          }}
        />
      ) : (
        <TaskLoading />
      )}
    </main>
  );
}
