import {
  Alert,
  Button,
  Chip,
  Label,
  ListBox,
  Select,
  Switch,
  Table,
} from "@heroui/react";
import {
  Activity,
  ChevronRight,
  Clock3,
  History,
  Power,
  TriangleAlert,
  Zap,
} from "lucide-react";
import { useRef, useState } from "react";

import { useLibraryDetailContext } from "./library-detail-context";
import {
  ChangeChip,
  ResultChip,
  SyncPageHeader,
  SyncRecordDetails,
  SyncSection,
} from "./library-sync-components";
import {
  formatSyncCount,
  incrementalAlerts,
  incrementalOverview,
  sampleMediaPath,
  triggerHistory,
} from "./library-sync-demo";

const intervals = ["15", "30", "60", "120"];
const batchLimits = ["100", "250", "500", "1000"];

function PollingOption({
  label,
  values,
  unit,
  value,
  onChange,
}: {
  label: string;
  values: string[];
  unit: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <Select
      className="grid grid-cols-[1fr_minmax(0,1fr)] items-center gap-2"
      selectedKey={value}
      onSelectionChange={(key) => {
        if (typeof key === "string" && values.includes(key)) onChange(key);
      }}
    >
      <Label>{label}</Label>
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {values.map((option) => (
            <ListBox.Item
              id={option}
              key={option}
              textValue={`${option} ${unit}`}
            >
              {option} {unit}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  );
}

function ListenerSwitch({
  label,
  value,
  onChange,
}: {
  label: string;
  value: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <Switch className="w-full" isSelected={value} onChange={onChange}>
      <Switch.Content className="flex w-full justify-between gap-3">
        <Label>{label}</Label>
        <Switch.Control>
          <Switch.Thumb />
        </Switch.Control>
      </Switch.Content>
    </Switch>
  );
}

const eventMetrics = [
  { label: "今日触发", value: incrementalOverview.total, color: "text-muted" },
  { label: "处理中", value: incrementalOverview.pending, color: "text-muted" },
  {
    label: "成功",
    value: incrementalOverview.succeeded,
    color: "text-success",
  },
  { label: "失败", value: incrementalOverview.failed, color: "text-danger" },
];

export function LibraryIncrementalSyncPage() {
  const { item, settings, setSettings } = useLibraryDetailContext();
  const [allTriggers, setAllTriggers] = useState(false);
  const [allAlerts, setAllAlerts] = useState(false);
  const [selectedAlert, setSelectedAlert] =
    useState<(typeof incrementalAlerts)[number]>();
  const detailsOrigin = useRef<HTMLElement | null>(null);
  const alertsToggle = useRef<HTMLButtonElement>(null);
  const triggers = allTriggers ? triggerHistory : triggerHistory.slice(0, 10);
  const alerts = allAlerts ? incrementalAlerts : incrementalAlerts.slice(0, 2);

  return (
    <div className="space-y-4">
      <SyncPageHeader
        title="增量同步"
        description="查看目录变更的演示记录，试用自动监听和批量处理设置。"
        icon={Zap}
        demonstration
      />
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        <SyncSection
          title="事件流状态"
          icon={Activity}
          action={
            <span role="status">
              <Chip
                color={settings.listening ? "success" : "default"}
                variant="soft"
                size="sm"
              >
                <span
                  aria-hidden="true"
                  className="size-1.5 rounded-full bg-current"
                />
                {settings.listening ? "监听中" : "已暂停监听"}
              </Chip>
            </span>
          }
        >
          <dl className="grid grid-cols-4 divide-x divide-divider py-2 text-center">
            {eventMetrics.map(({ label, value, color }) => (
              <div key={label} className="px-1">
                <dt className={`text-xs ${color}`}>{label}</dt>
                <dd className="text-xl font-bold tabular-nums">
                  {formatSyncCount(value)}
                </dd>
              </div>
            ))}
          </dl>
        </SyncSection>
        <SyncSection title="轮询配置" icon={Clock3}>
          <div className="grid gap-2">
            <PollingOption
              label="轮询间隔"
              values={intervals}
              unit="秒"
              value={settings.interval}
              onChange={(interval) =>
                setSettings((current) => ({ ...current, interval }))
              }
            />
            <PollingOption
              label="批量处理上限"
              values={batchLimits}
              unit="条"
              value={settings.batchLimit}
              onChange={(batchLimit) =>
                setSettings((current) => ({ ...current, batchLimit }))
              }
            />
          </div>
        </SyncSection>
        <SyncSection title="监听开关" icon={Power}>
          <div className="grid gap-3">
            <ListenerSwitch
              label="启用增量监听"
              value={settings.listening}
              onChange={(listening) =>
                setSettings((current) => ({ ...current, listening }))
              }
            />
            <ListenerSwitch
              label="包含目录变更"
              value={settings.includeDirectories}
              onChange={(includeDirectories) =>
                setSettings((current) => ({ ...current, includeDirectories }))
              }
            />
          </div>
        </SyncSection>
      </div>
      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
        <SyncSection
          title="最近触发记录"
          icon={History}
          action={
            <Button
              size="sm"
              variant="ghost"
              aria-expanded={allTriggers}
              onPress={() => setAllTriggers(!allTriggers)}
            >
              {allTriggers ? "收起" : "查看全部"}
              <ChevronRight aria-hidden="true" className="size-4" />
            </Button>
          }
        >
          <Table>
            <Table.ScrollContainer>
              <Table.Content
                aria-label="最近触发记录"
                className="min-w-[455px]"
              >
                <Table.Header>
                  <Table.Column>触发时间</Table.Column>
                  <Table.Column>事件类型</Table.Column>
                  <Table.Column isRowHeader>变更路径</Table.Column>
                  <Table.Column>处理结果</Table.Column>
                </Table.Header>
                <Table.Body>
                  {triggers.map((event) => (
                    <Table.Row key={event.time} id={event.time}>
                      <Table.Cell className="whitespace-nowrap tabular-nums">
                        今天 {event.time}
                      </Table.Cell>
                      <Table.Cell>
                        <ChangeChip kind={event.kind} />
                      </Table.Cell>
                      <Table.Cell className="max-w-44 wrap-anywhere">
                        {sampleMediaPath(item.directory_path, event.entry)}
                      </Table.Cell>
                      <Table.Cell>
                        <ResultChip result={event.result} />
                      </Table.Cell>
                    </Table.Row>
                  ))}
                </Table.Body>
              </Table.Content>
            </Table.ScrollContainer>
          </Table>
        </SyncSection>
        <SyncSection
          title="异常提示"
          icon={TriangleAlert}
          action={
            <Button
              ref={alertsToggle}
              size="sm"
              variant="ghost"
              aria-expanded={allAlerts}
              onPress={() => setAllAlerts(!allAlerts)}
            >
              {allAlerts ? "收起" : "查看全部"}
              <ChevronRight aria-hidden="true" className="size-4" />
            </Button>
          }
        >
          {selectedAlert && (
            <SyncRecordDetails
              key={selectedAlert.time}
              title="异常详情"
              closeLabel="返回异常列表"
              onClose={() => {
                setSelectedAlert(undefined);
                const origin = detailsOrigin.current;
                (origin?.isConnected ? origin : alertsToggle.current)?.focus();
              }}
            >
              <p className="font-medium">{selectedAlert.title}</p>
              <p className="mt-1 text-sm text-muted">{selectedAlert.summary}</p>
              <p className="mt-2 text-sm">{selectedAlert.instruction}</p>
            </SyncRecordDetails>
          )}
          <ul className="space-y-3">
            {alerts.map((alert) => (
              <li key={alert.time}>
                <Alert status={alert.severity}>
                  <Alert.Indicator />
                  <Alert.Content>
                    <Alert.Title>
                      <span className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                        {alert.title}
                        <span className="text-xs font-normal text-muted">
                          今天 {alert.time}
                        </span>
                      </span>
                    </Alert.Title>
                    <Alert.Description>{alert.summary}</Alert.Description>
                    <Button
                      size="sm"
                      variant="secondary"
                      className="mt-2 self-start"
                      onPress={({ target }) => {
                        detailsOrigin.current =
                          target instanceof HTMLElement ? target : null;
                        setSelectedAlert(alert);
                      }}
                    >
                      去处理
                    </Button>
                  </Alert.Content>
                </Alert>
              </li>
            ))}
          </ul>
        </SyncSection>
      </div>
    </div>
  );
}
