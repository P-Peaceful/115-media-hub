import type { components } from "@/lib/api/generated/schema";

export type TaskEvent = components["schemas"]["TaskEvent"];
export type TaskEventKind = "task.changed" | "schedule.changed";

export function decodeTaskEvent(event: Event): TaskEvent | null {
  if (!(event instanceof MessageEvent) || typeof event.data !== "string")
    return null;
  try {
    const value: unknown = JSON.parse(event.data);
    if (
      typeof value !== "object" ||
      value === null ||
      !("id" in value) ||
      typeof value.id !== "string" ||
      value.id.length === 0
    )
      return null;
    if (!("notification" in value)) return { id: value.id };
    const notification = value.notification;
    if (
      notification === "success" ||
      notification === "failure" ||
      notification === "partial_failure"
    ) {
      return { id: value.id, notification };
    }
    return null;
  } catch {
    return null;
  }
}
