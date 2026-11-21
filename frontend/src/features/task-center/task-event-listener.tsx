import { Link, Toast } from "@heroui/react";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { decodeTaskEvent } from "@/lib/events/task-events";

import { scheduleKeys, taskKeys } from "./queries";
import { outcomes } from "./task-kinds";

export function TaskEventListener() {
  const client = useQueryClient();
  const [showNotifications, setShowNotifications] = useState(false);
  useEffect(() => {
    if (typeof EventSource === "undefined") return;
    const stream = new EventSource("/api/v1/events");
    let timer: number | undefined;
    let tasksChanged = false;
    let schedulesChanged = false;
    const changedTasks = new Set<string>();

    function refresh() {
      timer = undefined;
      if (tasksChanged)
        void client.invalidateQueries({ queryKey: taskKeys.lists });
      if (schedulesChanged)
        void client.invalidateQueries({ queryKey: scheduleKeys.all });
      for (const id of changedTasks) {
        void client.invalidateQueries({ queryKey: taskKeys.detail(id) });
      }
      tasksChanged = false;
      schedulesChanged = false;
      changedTasks.clear();
    }
    function delayRefresh() {
      timer ??= window.setTimeout(refresh, 250);
    }
    function onOpen() {
      void client.invalidateQueries({ queryKey: taskKeys.all });
      void client.invalidateQueries({ queryKey: scheduleKeys.all });
    }
    function onTaskChange(event: Event) {
      const info = decodeTaskEvent(event);
      if (!info) return;
      changedTasks.add(info.id);
      tasksChanged = true;
      delayRefresh();
      if (info.notification) {
        setShowNotifications(true);
        Toast.toast(outcomes[info.notification], {
          variant:
            info.notification === "success"
              ? "success"
              : info.notification === "failure"
                ? "danger"
                : "warning",
          description: (
            <Link href={`/tasks/${encodeURIComponent(info.id)}`}>查看任务</Link>
          ),
          timeout: 7000,
        });
      }
    }
    function onScheduleChange(event: Event) {
      if (!decodeTaskEvent(event)) return;
      schedulesChanged = true;
      delayRefresh();
    }
    stream.addEventListener("open", onOpen);
    stream.addEventListener("task.changed", onTaskChange);
    stream.addEventListener("schedule.changed", onScheduleChange);
    return () => {
      stream.close();
      if (timer !== undefined) window.clearTimeout(timer);
      Toast.toast.clear();
    };
  }, [client]);

  return showNotifications ? <Toast.Provider placement="bottom end" /> : null;
}
