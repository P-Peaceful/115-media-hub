# Type Safety and Request Contracts

## 1. Scope / Trigger

Read for API, DTO, request or external-data changes. [Project context](../project-context.md) defines the current health-only contract. [tsconfig.json](../../../frontend/tsconfig.json) enables strict mode, unused-local/parameter checks, isolated modules and no emit. Runtime validation is a separate concern.

## 2. Signatures

[client.ts](../../../frontend/src/shared/api/client.ts) contains the concrete boundary:

```ts
export class ApiError extends Error {
  constructor(public readonly status: number)
}
type RequestOptions = RequestInit & { csrfToken?: string }
export async function request<T = unknown>(
  path: string, options: RequestOptions = {},
): Promise<T | undefined>
```

The constructor signature above is abbreviated; the implementation sets message `HTTP ${status}` and name `ApiError`. This is a status-only error, not a parsed error envelope.

## 3. Contracts

[OpenAPI](../../../api/openapi.yaml) generates [schema.d.ts](../../../frontend/src/shared/api/generated/schema.d.ts) through `make generate`. It currently defines health `status: 'ok' | 'unavailable'` only. These are compile-time declarations; `response.json() as Promise<T>` does not verify the server body. No Zod/Yup/runtime schema library or validation adapter is installed in [package.json](../../../frontend/package.json).

Use same-origin absolute paths with a single leading `/`. Before fetch, the current guard requires a leading slash and an origin of `http://same-origin.invalid` when resolved against that synthetic base. It rejects ordinary external/protocol-relative URLs, but `//same-origin.invalid/...` passes that guard; do not treat it as blanket rejection of every protocol-relative input. The helper normalizes the method to uppercase, sets `Accept: application/json`, and forces `credentials: 'same-origin'`. `RequestInit` passes options such as body and signal; JSON serialization and Content-Type are the caller's responsibility. A supplied `csrfToken` becomes `X-CSRF-Token` only for methods outside GET/HEAD/OPTIONS. This is client header support only: token acquisition, server sessions/authentication and CSRF validation do not exist.

## 4. Validation & Error Matrix

| Input / response | Result |
| --- | --- |
| `'/health/live'`, successful JSON | Parsed value typed by caller, without runtime validation |
| No leading slash, or URL resolving to a different origin than the synthetic base | Error before fetch |
| Non-2xx response | `ApiError(status)`; body is not exposed |
| Successful 204 or HEAD | `undefined`; never parse JSON |
| Other successful response with invalid/empty JSON | JSON parsing rejects |
| Fetch failure/abort | Fetch rejection propagates; no ApiError conversion |
| Mutation without token | No CSRF header automatically added |

Only `MEDIA_HUB_DEV_API_URL` in [vite.config.ts](../../../frontend/vite.config.ts) sets the development proxy target; the browser request helper has no configurable external API base. Keep same-origin production requests consistent with server routing.

## 5. Good/Base/Bad Cases

Good: consume a health response with an explicit generated type and handle the possible `undefined`. Base: `request('/health/live')` returns `unknown | undefined` until narrowed. Bad: cast a remote payload to a domain type and treat it as validated, assume every successful response has JSON, or send a guessed CSRF token. Generics communicate expectations; they do not establish trust.

## 6. Tests Required

[client.test.ts](../../../frontend/src/shared/api/client.test.ts) asserts JSON, cookie credentials, 204, status-only errors, explicit mutation CSRF and external URL rejection. For changes assert fetch arguments, HEAD/no-body behavior, safe-method CSRF omission, parse failure and abort behavior where affected; those latter cases are not all covered today. Run `pnpm --dir frontend typecheck`, `pnpm --dir frontend test`, and `make check` before completion.

## 7. Wrong vs Correct

Wrong: edit generated declarations, use `any` to hide a mismatch, or dereference a `request<T>` result without considering `undefined`. Correct: update OpenAPI/regenerate for contract changes, narrow unknown values, and distinguish no-content from data. New untrusted payload boundaries need an explicit validation design and tests; do not claim a runtime validator already exists.
