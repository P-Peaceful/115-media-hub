# Type Safety

## Compiler and type ownership

Keep TypeScript `strict`, `noUncheckedIndexedAccess`, and `exactOptionalPropertyTypes` enabled; do not weaken checks to resolve application errors. Use `import type`, inferred local types where clear, and explicit exported API/prop contracts. Prefer string-literal unions/discriminated unions for finite states. Do not hand-maintain backend DTO copies.

Generate transport types from `api/openapi.yaml` into `frontend/src/lib/api/generated/`. Local UI/view models remain feature-owned; map transport snake_case to camelCase only at an explicit boundary when needed. Keep opaque IDs as strings; preserve absent/null distinctions and UTC time strings from the [baseline](../guides/project-baseline.md#shared-http-contract-decisions).

## Runtime validation

TypeScript types disappear at runtime. Treat network JSON, SSE payloads, persisted preferences, and URL parameters as untrusted values. The API/event boundary owns decoding into validated types. Generated types alone do not validate responses. Use small explicit guards for simple payloads; if a schema validator becomes necessary, choose one consistent implementation in the initialization/feature task rather than introducing several libraries.

Illustrative narrowing for a deliberately small event shape (the actual event union must be defined in OpenAPI):

```ts
type JobStatusChanged = {
  job_id: string;
  status: "queued" | "running" | "succeeded" | "failed" | "canceled";
};

export function isJobStatusChanged(value: unknown): value is JobStatusChanged {
  if (typeof value !== "object" || value === null) return false;
  if (!("job_id" in value) || typeof value.job_id !== "string") return false;
  if (!("status" in value)) return false;
  return (
    value.status === "queued" ||
    value.status === "running" ||
    value.status === "succeeded" ||
    value.status === "failed" ||
    value.status === "canceled"
  );
}
```

The statuses above illustrate a closed union; the job design must settle its full state machine (including cancellation/recovery) before generating production types. Guards must validate every field consumed and distinguish unknown event kinds from malformed known ones.

## Errors and prohibited shortcuts

Catch errors as `unknown` and narrow them through a shared API error type. Map stable backend codes to actionable UI messages; preserve `request_id` for diagnostics without displaying internal error details. Handle invalid JSON, non-JSON errors, empty success responses, and cancellation explicitly in the HTTP client.

Avoid `any`, double assertions (`as unknown as ...`), non-null assertions for unchecked data, `@ts-ignore`, and casting JSON directly to a DTO. `as const` is appropriate for literal/query-key inference. A narrow `@ts-expect-error` requires an explained external typing limitation and a follow-up condition; it cannot replace validation.

Reference: [TypeScript strict](https://www.typescriptlang.org/tsconfig/strict.html).
