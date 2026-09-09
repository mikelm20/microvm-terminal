# learn-platform contracts (frozen interfaces, v1)

Updated 2026-04-21: client surface pivoted from mobile-native to web-first per ICP v2 + product brief v2. HTTP, WebSocket, Postgres and lesson-YAML contracts in sections 4 to 7 are unchanged; only sections 1, 2, 3 and 8 were rewritten to reflect the surface change.

Source of truth for every parallel agent. If a contract below is ambiguous or incomplete, STOP and open an issue labeled `contracts-question`. Do NOT improvise. Contract drift is the biggest risk of parallel builds.

This document freezes:

1. Mission + stack choices
2. Monorepo layout (who writes where)
3. Shared packages: api, tokens, voice, lessons
4. HTTP endpoints (request/response shapes)
5. WebSocket event bus (every event type + payload)
6. Postgres schema outline
7. Lesson YAML schema extensions
8. Error contract
9. Per-agent scope + exit criteria
10. Conventions (branches, commits, secrets)

---

## 1. Mission

Web app (desktop plus mobile responsive) that teaches Claude Code by embedding it. The learner opens the page, anonymously enters a three-phase arc (Fundamentos concepts, Terminal guiada practice, Autonomia handoff), and at phase two sends a real prompt to a real Claude Code instance running in a server-side Firecracker VM. Tool-call cards stream in next to the terminal as the wow moment; when a predicate matches, the wizard advances. The arc moves from scaffolded commands on day 1 to free composition on day N. End state: the employee downloads Claude Code locally and the company pays the Anthropic license.

Wow moment, textual: "el primer prompt con tool calls visibles como protagonistas". Every architectural choice serves that moment.

Audience: employees, technical and non-technical, in companies adopting Claude Code but lacking internal capacity to train. Many open a terminal for the first time in their career. See `CLAUDE.md` at the repo root and `<drive-link>` in the team drive for the full ICP. Do not use the word "graduation" in user-facing copy or in contracts.

## 2. Stack choices (frozen, do not renegotiate)

- **Web app**: Next.js 15 App Router + Tailwind + xterm.js for the embedded terminal, consuming `shared/*` packages. Lives in `apps/web/` in this monorepo. Self-hosted behind Caddy on `learn-01`. Caddy routes a path prefix under `learn.example.com` (exact prefix TBD at Fase 1 deploy) to this surface; the landing owns the root.
- **Public landing**: separate repo `learn-landing` (Next.js 15 + Tailwind + `next/og`), its own Komodo stack. Serves the root of `learn.example.com`. Imports `@learn/shared-*` for contract and brand parity.
- **Mobile native**: deferred. A native rewrite is not in scope for the first vendible milestone; the web app is mobile-responsive for the non-desktop audience.
- **Control plane**: Go 1.22 (existing), chi router (existing), Postgres 16, sqlc for typed queries, goose for migrations.
- **`claude-wrap` + guest agent**: Go 1.22, static binary, communicates via vsock (existing protocol) + JSON event payloads.
- **Platform-owned Anthropic proxy**: Go 1.22, standalone binary, per-session quota, audit log.
- **Package manager**: pnpm + Turborepo. TypeScript 5.6+ strict. Zod for runtime validation at every boundary.
- **Auth**: magic-link via Resend. Passkeys as future upgrade path. Anonymous UUID from first launch.
- **Secrets**: Doppler, injected via `doppler run`. Nothing in `/etc/learn-platform/` anymore.
- **Telemetry**: PostHog Cloud EU (free tier) + Sentry (free tier). Single `track()` wrapper in each client.
- **Tests**: integration-first via learner-bot harness + fixture corpus. No unit-tests-for-the-sake-of-it.
- **CI**: GitHub Actions. Required checks: lint, typecheck, build, integration tests (where they don't need a live Claude).

## 3. Monorepo layout

```
/
  CONTRACTS.md                 (this doc)
  CLAUDE.md                    (project root agent instructions)
  README.md
  package.json                 (root, private, pnpm workspace orchestrator)
  pnpm-workspace.yaml
  turbo.json
  tsconfig.base.json           (shared TS config)
  .nvmrc                       (node 20 LTS)
  .gitignore

  apps/
    web/                       (Next.js 15 App Router. Learner surface. Agent-Web owns once scaffolded.)
      app/                     (App Router routes: phases, lesson, terminal, wizard)
      components/
      lib/                     (API client, WS clients, xterm wiring)
      public/
      package.json
      tailwind.config.js       (presets: @learn/shared-tokens/tailwind-preset)
      tsconfig.json
  (The marketing/proof landing lives in its own repo: learn-landing.
  Mobile native is deferred; the web app is responsive.)

  shared/                      (TS packages, imported as @learn/shared/*)
    api/                       (Zod schemas + TS types for HTTP + WS)
      package.json
      schemas.ts               (all HTTP request/response schemas)
      events.ts                (WS event union)
      errors.ts                (error contract)
      index.ts
    tokens/                    (design tokens, source of truth in tokens.json)
      package.json
      tokens.json              (FROZEN. Do not edit without architecture review.)
      tokens.ts                (typed TS re-export for web consumers)
      tailwind-preset.js       (generated; consumed by apps/*/tailwind.config.js)
      index.ts
    voice/                     (copy tables)
      package.json
      es.json                  (Spanish voice, canonical)
      en.json                  (English voice, translation)
      types.ts                 (typed key enum derived from es.json)
      index.ts
    lessons/                   (Lesson TS schema + loader)
      package.json
      schema.ts                (Zod schema for Lesson YAML)
      loader.ts                (YAML -> typed Lesson)
      index.ts

  control-plane/               (Go, existing, extended.)
    cmd/learn-cp/main.go
    internal/api/              (HTTP+WS handlers. Agent-API owns.)
    internal/session/          (VM lifecycle + transcript + warm pool. Agent-API owns.)
    internal/identity/         (UUID + auth. Agent-API owns.)
    internal/lesson/           (Server-side predicate eval. Agent-Spine owns.)
    internal/vm/               (Firecracker + Jailer wrapper. Agent-Gate owns.)
    internal/mail/             (Resend. Agent-API owns.)
    internal/db/
      migrations/              (goose. Agent-API owns.)
      queries/                 (sqlc. Agent-API owns.)
    go.mod, go.sum

  guest-agent/                 (Go, inside each VM. Extended by Agent-Spine.)
    cmd/learn-guest-agent/main.go

  vm-image/                    (rootfs build. Existing.)
    Dockerfile
    learn-shell.sh             (existing; Agent-Spine swaps `claude` for `claude-wrap`)
    claude-wrap/               (NEW, Go binary. Agent-Spine owns.)
      cmd/claude-wrap/main.go
      internal/parser/         (parses claude --output-format stream-json)
      internal/emitter/        (emits to vsock socket)
    sandbox/empresa-prueba/    (content, expanded by Agent-Spine)

  lessons/                     (YAML. Agent-Spine owns.)
    m2-primera-conversacion.{es,en}.yml
    m3-organizar-cabeza.{es,en}.yml
    ...
    hello-claude.yml

  infra/                       (host provisioning. Agent-Gate owns.)
    bootstrap.sh
    systemd/
    nftables/
    jailer/
    Caddyfile
    docker-compose.dev.yml     (NEW. Local dev stack: Postgres + control plane + 1 VM.)

  proxy/                       (NEW. Platform-owned Anthropic proxy. Agent-Gate owns.)
    cmd/claude-proxy/main.go
    internal/quota/
    internal/audit/

  tests/                       (Agent-Spine owns.)
    fixtures/claude/           (captured `claude --print` outputs, one per lesson step)
    learner-bot/               (harness that runs each lesson against real Claude, verifies predicate events)

  web/                         (LEGACY old MVP. Read-only reference. Deleted at final merge.)
  phase-0-spike/               (historical, keep.)
```

File ownership rules:
- If an agent needs a file outside their scope, they read it, do not write.
- Shared mutation targets: `package.json` (root), `turbo.json`, `CONTRACTS.md`. Only Agent-Tooling writes these post-scaffold.
- `CLAUDE.md` and `README.md` are append-only during the 2-day sprint; reconcile at final merge.

## 4. Shared packages

### 4.1 `@learn/shared-api/schemas.ts`

Every HTTP endpoint has a Zod request schema and a Zod response schema. Both clients and server import from the same source. Server validates on request receipt; clients validate on response receipt.

Canonical example (Agent-Tooling fills in the rest):

```ts
import { z } from "zod";

export const LangSchema = z.enum(["es", "en"]);
export type Lang = z.infer<typeof LangSchema>;

export const DepartmentSchema = z.enum([
  "ventas", "marketing", "tecnologia", "producto",
  "finanzas", "rrhh", "legal", "estrategia",
]);
export type Department = z.infer<typeof DepartmentSchema>;

// POST /sessions
export const CreateSessionRequest = z.object({
  lesson_id: z.string(),
  lang: LangSchema,
  warm: z.boolean().optional(),
});
export const CreateSessionResponse = z.object({
  session_id: z.string().uuid(),
  vm_ip: z.string(),
  pty_ws_url: z.string().url(),
  wizard_ws_url: z.string().url(),
  preview_url_template: z.string(),
});

// POST /identity
export const MintIdentityRequest = z.object({
  uuid: z.string().uuid().optional(), // if present, re-attest
  device_hint: z.string().optional(),
  lang: LangSchema,
});
export const MintIdentityResponse = z.object({
  uuid: z.string().uuid(),
  cookie: z.string(), // signed cookie to set
});

// POST /auth/magic-link
export const RequestMagicLinkRequest = z.object({
  email: z.string().email(),
  anonymous_uuid: z.string().uuid().optional(),
  lang: LangSchema,
});
export const RequestMagicLinkResponse = z.object({
  ok: z.literal(true),
  expires_in_seconds: z.number(),
});

// POST /auth/claim
export const ClaimAccountRequest = z.object({
  magic_link_token: z.string(),
  anonymous_uuid: z.string().uuid().optional(),
});
export const ClaimAccountResponse = z.object({
  uuid: z.string().uuid(),
  email: z.string().email(),
  session_token: z.string(),
});

// GET /me
export const MeResponse = z.object({
  uuid: z.string().uuid(),
  email: z.string().email().nullable(),
  lang: LangSchema,
  department: DepartmentSchema.nullable(),
  streak_days: z.number(),
  total_xp: z.number(),
  grace_tokens: z.number(),
  created_at: z.string(),
  claimed_at: z.string().nullable(),
});

// PATCH /me
export const PatchMeRequest = z.object({
  lang: LangSchema.optional(),
  department: DepartmentSchema.optional(),
  name: z.string().max(64).optional(),
  haptics_enabled: z.boolean().optional(),
  push_enabled: z.boolean().optional(),
});
export const PatchMeResponse = MeResponse;

// POST /progress/sync
export const ProgressSyncRequest = z.object({
  uuid: z.string().uuid(),
  state: z.object({
    modules: z.record(z.string(), z.object({
      completed_steps: z.array(z.string()),
      paused_at: z.string().nullable(),
      xp_earned: z.number(),
    })),
    streak_days: z.number(),
    grace_tokens: z.number(),
    last_visited: z.string(),
    seen_coachmarks: z.array(z.string()),
    updated_at: z.string(),
  }),
});
export const ProgressSyncResponse = z.object({
  state: ProgressSyncRequest.shape.state,
  server_updated_at: z.string(),
});

// GET /lessons
export const ListLessonsResponse = z.object({
  lessons: z.array(z.object({
    id: z.string(),
    module_number: z.number(),
    language: LangSchema,
    title: z.string(),
    subtitle: z.string().optional(),
    estimated_minutes: z.number().optional(),
  })),
});

// GET /sessions/{id}/heartbeat
export const HeartbeatResponse = z.object({
  session_id: z.string().uuid(),
  vm_ip: z.string(),
  alive: z.boolean(),
  claude_busy: z.boolean(),
  last_event_at: z.string(),
  age_seconds: z.number(),
});

// GET /sessions/{id}/transcript
export const TranscriptResponse = z.object({
  session_id: z.string().uuid(),
  events: z.array(z.unknown()), // typed as WsEvent[] in events.ts
});

// POST /sessions/{id}/prompt  (optimistic / offline queue sync)
export const SubmitPromptRequest = z.object({
  text: z.string().min(1).max(4096),
  client_ts: z.string(),
});
// Idempotency-Key header required (UUID).

// POST /sessions/{id}/attach  (camera/image)
// multipart/form-data, field "image" with JPEG/PNG. No Zod.
```

Full schemas file is Agent-Tooling's deliverable. Clients import `CreateSessionRequest`, etc. Server handlers validate with `.parse()`.

### 4.2 `@learn/shared-api/events.ts`

Every event on the wizard WebSocket. Typed discriminated union. Guest-agent emits, control plane forwards, clients consume.

```ts
import { z } from "zod";

const BaseEvent = z.object({
  ts: z.string(), // ISO 8601
});

export const SessionStartedEvent = BaseEvent.extend({
  type: z.literal("session_started"),
  session_id: z.string().uuid(),
  lesson_id: z.string(),
});

export const VmBootingEvent = BaseEvent.extend({
  type: z.literal("vm_booting"),
  stage: z.enum(["jailer", "kernel", "rootfs", "guest_agent", "claude"]),
});

export const VmReadyEvent = BaseEvent.extend({
  type: z.literal("vm_ready"),
});

export const AgentOnlineEvent = BaseEvent.extend({
  type: z.literal("agent_online"),
});

export const ProcessStartedEvent = BaseEvent.extend({
  type: z.literal("process_started"),
  name: z.string(),
});

export const PortListeningEvent = BaseEvent.extend({
  type: z.literal("port_listening"),
  port: z.number(),
});

export const ClaudePromptSentEvent = BaseEvent.extend({
  type: z.literal("claude_prompt_sent"),
  text: z.string(),
  length: z.number(),
  locale_detected: z.string().optional(),
});

export const ClaudeToolCallEvent = BaseEvent.extend({
  type: z.literal("claude_tool_call"),
  tool: z.enum(["Read", "Write", "Edit", "Bash", "Glob", "Grep", "Task", "WebFetch", "WebSearch", "SlashCommand", "Other"]),
  args: z.record(z.string(), z.string()),
  path: z.string().optional(),
  command: z.string().optional(),
  subagent_type: z.string().optional(),
  call_id: z.string(),
});

export const ClaudeToolResultEvent = BaseEvent.extend({
  type: z.literal("claude_tool_result"),
  call_id: z.string(),
  ok: z.boolean(),
  summary: z.string().optional(), // first line / truncated preview
});

export const ClaudeSlashCommandEvent = BaseEvent.extend({
  type: z.literal("claude_slash_command"),
  command: z.string(),
  args: z.string().optional(),
});

export const ClaudeMessageEvent = BaseEvent.extend({
  type: z.literal("claude_message"),
  role: z.enum(["user", "assistant", "system"]),
  text: z.string(),
  duration_ms: z.number().optional(),
  turn_id: z.string(),
});

export const ClaudeBusyEvent = BaseEvent.extend({
  type: z.literal("claude_busy"),
  busy: z.boolean(),
});

export const ClaudeThinkingEvent = BaseEvent.extend({
  type: z.literal("claude_thinking"),
  chars_so_far: z.number(),
});

export const ClaudeTokenStreamedEvent = BaseEvent.extend({
  type: z.literal("claude_token_streamed"),
  turn_id: z.string(),
  delta: z.string(),
  total_chars: z.number(),
});

export const FileExistsEvent = BaseEvent.extend({
  type: z.literal("file_exists"),
  path: z.string(),
});

export const FileContentsRegexEvent = BaseEvent.extend({
  type: z.literal("file_contents_regex"),
  path: z.string(),
  regex_id: z.string(),
  snippet: z.string(),
});

export const StepSatisfiedEvent = BaseEvent.extend({
  type: z.literal("step_satisfied"),
  step_id: z.string(),
  lesson_id: z.string(),
  evidence: z.unknown(), // the event that satisfied the predicate
});

export const SessionErrorEvent = BaseEvent.extend({
  type: z.literal("session_error"),
  code: z.string(),
  message: z.string(),
});

export const SessionClosedEvent = BaseEvent.extend({
  type: z.literal("session_closed"),
  reason: z.enum(["user_closed", "grace_expired", "reaped", "crashed"]),
});

export const WsEvent = z.discriminatedUnion("type", [
  SessionStartedEvent, VmBootingEvent, VmReadyEvent, AgentOnlineEvent,
  ProcessStartedEvent, PortListeningEvent,
  ClaudePromptSentEvent, ClaudeToolCallEvent, ClaudeToolResultEvent,
  ClaudeSlashCommandEvent, ClaudeMessageEvent, ClaudeBusyEvent,
  ClaudeThinkingEvent, ClaudeTokenStreamedEvent,
  FileExistsEvent, FileContentsRegexEvent,
  StepSatisfiedEvent, SessionErrorEvent, SessionClosedEvent,
]);
export type WsEvent = z.infer<typeof WsEvent>;
```

The control plane persists every `WsEvent` to Postgres (`session_events` table) and replays on WS reconnect. This is the transcript.

### 4.3 `@learn/shared-api/errors.ts`

Every non-2xx response from the control plane returns this shape:

```ts
export const ApiError = z.object({
  code: z.enum([
    "capacity_full",
    "auth_required",
    "auth_invalid",
    "magic_link_expired",
    "vm_launch_failed",
    "vm_not_found",
    "session_reaped",
    "bad_request",
    "not_found",
    "rate_limited",
    "internal",
  ]),
  message: z.string(),
  retry_after_seconds: z.number().optional(),
  queue_position: z.number().optional(),
  request_id: z.string(),
});
export type ApiError = z.infer<typeof ApiError>;
```

Clients map `code` to a localized voice-table entry, never show `message` to users.

### 4.4 `@learn/shared-tokens`

The JSON file `tokens.json` is the ONLY source of truth for colors, type, spacing, motion, shadow, radius. Do not hard-code values anywhere else. Agent-Tooling emits:

- `tokens.ts` (typed TS export)
- `tailwind-preset.js` (consumed by the separate marketing/proof web repo; the future mobile rewrite will consume it too)

Initial values are committed at scaffold time. Evolving requires architecture review.

### 4.5 `@learn/shared-voice`

`es.json` is canonical. `en.json` is a translation that mirrors the key tree. `types.ts` is generated from `es.json` keys to enforce completeness at compile time.

Top-level keys (Agent-Tooling fills the shape; content mainly set by Agent-Spine and Agent-Convo):

```
welcome.*
auth.*
path.*
lesson.loading.*
lesson.context.*
lesson.composer.*
lesson.moment.*
lesson.error.*
return.*
settings.*
certificate.*
share.*
push.*
error.capacity
error.launch
error.unknown
error.auth
```

Forbidden words in any voice entry: `sandbox`, `VM`, `agent` (meaning the sandbox agent; `Claude` is fine), `WebSocket`, `PTY`, `vsock`, `prompt template`, `workflow`, em dashes.

### 4.6 `@learn/shared-lessons/schema.ts`

Extends the existing Predicate union from `web/lib/lessons.ts` with:

```ts
export const RoleVariantSchema = z.object({
  body: z.string(),
  prompt: z.string().optional(),
  hint: z.string().optional(),
});

export const LessonStepSchema = z.object({
  id: z.string(),
  title: z.string(),
  body: z.string().optional(),
  prompt: z.string().optional(),
  hint: z.string().optional(),
  xp: z.number(),
  success: PredicateSchema.optional(),
  success_title: z.string().optional(),
  success_sub: z.string().optional(),
  // NEW
  expected_response_seconds: z.number().optional(), // default 60
  voice_first: z.boolean().optional(),              // composer defaults to mic
  requires_camera: z.boolean().optional(),          // composer shows camera CTA
  role_variants: z.record(DepartmentSchema, RoleVariantSchema).optional(),
  // Per-step priming passed to claude via --append-system-prompt. Used by
  // F2 Lab substeps where the canonical answer must be deterministic.
  system_prompt: z.string().optional(),
});

export const PredicateSchema = z.discriminatedUnion("type", [
  z.object({ type: z.literal("agent_online") }),
  z.object({ type: z.literal("process_started"), match: z.object({ name: z.string() }) }),
  z.object({ type: z.literal("port_listening"), match: z.object({ port: z.number().optional() }).optional() }),
  z.object({ type: z.literal("claude_prompt_sent"), match_regex: z.string().optional() }),
  z.object({ type: z.literal("claude_tool_call"), match: z.object({
    tool: z.string(),
    path_regex: z.string().optional(),
    command_regex: z.string().optional(),
    subagent_type: z.string().optional(),
  }) }),
  z.object({ type: z.literal("claude_slash_command"), match: z.object({ command: z.string() }) }),
  z.object({ type: z.literal("file_exists"), match: z.object({ path: z.string() }) }),
  z.object({ type: z.literal("file_contents_regex"), match: z.object({ path: z.string(), regex: z.string() }) }),
]);
```

Lesson YAMLs mirror this schema. `role_variants.ventas.body` overrides `body` when the learner's department is `ventas`.

## 5. HTTP endpoints

All endpoints live under `https://api.learn.example.com` (prod) or `http://localhost:8080` (dev). JSON unless stated. Auth: `Cookie: learn_session=<signed>` for authenticated routes; anonymous UUID cookie for everything else.

| Method | Path | Request | Response | Auth | Owner |
|---|---|---|---|---|---|
| POST | `/identity` | `MintIdentityRequest` | `MintIdentityResponse` | none | Agent-API |
| POST | `/auth/magic-link` | `RequestMagicLinkRequest` | `RequestMagicLinkResponse` | anon | Agent-API |
| POST | `/auth/claim` | `ClaimAccountRequest` | `ClaimAccountResponse` | none (token in body) | Agent-API |
| GET | `/me` | - | `MeResponse` | session | Agent-API |
| PATCH | `/me` | `PatchMeRequest` | `PatchMeResponse` | session | Agent-API |
| GET | `/lessons` | - | `ListLessonsResponse` | anon/session | Agent-API |
| GET | `/lessons/:id` | - | Full `Lesson` per schema | anon/session | Agent-API |
| POST | `/progress/sync` | `ProgressSyncRequest` | `ProgressSyncResponse` | session or UUID | Agent-API |
| GET | `/progress` | - | same as sync state | session or UUID | Agent-API |
| POST | `/sessions` | `CreateSessionRequest` | `CreateSessionResponse` | session or UUID | Agent-API |
| GET | `/sessions/:id` | - | session state | owner | Agent-API |
| GET | `/sessions/:id/heartbeat` | - | `HeartbeatResponse` | owner | Agent-API |
| GET | `/sessions/:id/transcript` | - | `TranscriptResponse` | owner | Agent-API |
| POST | `/sessions/:id/prompt` | `SubmitPromptRequest`, header `Idempotency-Key` | `{ ok: true, turn_id }` | owner | Agent-API |
| POST | `/sessions/:id/attach` | multipart image | `{ inbox_path: string }` | owner | Agent-API |
| DELETE | `/sessions/:id` | - | `{ ok: true }` | owner | Agent-API |
| WS | `/sessions/:id/ws` | - | stream of `WsEvent` | owner | Agent-API |
| WS | `/sessions/:id/pty` | - | raw bytes (desktop escape hatch; optional) | owner | Agent-API |
| GET | `/healthz` | - | `{ ok: true }` | none | Agent-API |
| GET | `/metrics` | - | Prometheus text | internal | Agent-API |

Public marketing/proof web (landing, `/p/[uuid]`, `/certificate/[id]`, `/verify/[id]`, OG cards) lives in a separate repo (`learn-landing`) on its own VPS. This monorepo only exposes the read-only control-plane endpoints that the landing repo consumes: `GET /p/:uuid/public`, `GET /certificate/:id/public`. Agent-API ships those alongside the authenticated surface above.

Landing and app are intentionally decoupled: no shared deployment, no shared domain dependency.

## 6. Postgres schema outline

Agent-API writes full goose migrations matching this outline. sqlc generates the Go query layer.

```sql
-- 001_identities.up.sql
create table identities (
  uuid uuid primary key,
  email text unique,
  email_verified_at timestamptz,
  lang text not null default 'es',
  department text,
  name text,
  haptics_enabled boolean not null default false,
  push_enabled boolean not null default false,
  created_at timestamptz not null default now(),
  claimed_at timestamptz,
  last_seen_at timestamptz not null default now()
);
create index identities_email_idx on identities (email);

-- 002_sessions.up.sql
create table sessions (
  id uuid primary key,
  identity_uuid uuid not null references identities(uuid),
  lesson_id text not null,
  lang text not null,
  department text,
  vm_ip inet,
  warm boolean not null default false,
  created_at timestamptz not null default now(),
  ready_at timestamptz,
  reaped_at timestamptz,
  grace_started_at timestamptz,
  last_event_at timestamptz
);
create index sessions_identity_idx on sessions (identity_uuid);
create index sessions_alive_idx on sessions (reaped_at) where reaped_at is null;

-- 003_session_events.up.sql
create table session_events (
  session_id uuid not null references sessions(id) on delete cascade,
  seq bigint generated always as identity,
  ts timestamptz not null default now(),
  type text not null,
  payload jsonb not null,
  primary key (session_id, seq)
);
create index session_events_ts_idx on session_events (session_id, ts);

-- 004_progress.up.sql
create table progress (
  identity_uuid uuid primary key references identities(uuid),
  state jsonb not null,
  updated_at timestamptz not null default now()
);

-- 005_magic_links.up.sql
create table magic_links (
  token text primary key,
  email text not null,
  anonymous_uuid uuid,
  lang text not null,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  consumed_at timestamptz
);
create index magic_links_email_idx on magic_links (email);

-- 006_auth_sessions.up.sql
create table auth_sessions (
  token text primary key,
  identity_uuid uuid not null references identities(uuid) on delete cascade,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  last_used_at timestamptz not null default now()
);
create index auth_sessions_identity_idx on auth_sessions (identity_uuid);

-- 007_certificates.up.sql
create table certificates (
  id text primary key, -- hash(uuid, lesson_id, completed_at)
  identity_uuid uuid not null references identities(uuid),
  lesson_id text not null,
  completed_at timestamptz not null,
  signature text not null, -- Ed25519 signature
  created_at timestamptz not null default now()
);
```

## 7. Error contract

See section 4.3. Every handler returns either a 2xx with the typed response or a 4xx/5xx with `ApiError`. Request ID from `X-Request-Id` header (generated server-side if missing) is echoed in both success and error responses.

## 8. Per-agent scope + exit criteria

### Agent-Tooling

**Scope**: monorepo glue + codegen + CI + local dev infra.

**Owns**:
- `shared/api/schemas.ts`, `events.ts`, `errors.ts`, `index.ts` (full Zod impls)
- `shared/tokens/tokens.ts` + `tailwind-preset.js` (generators from `tokens.json`)
- `shared/voice/types.ts` (generated enum from `es.json` keys)
- `shared/lessons/schema.ts`, `loader.ts`
- Root `package.json`, `pnpm-workspace.yaml`, `turbo.json`, `tsconfig.base.json`
- `.github/workflows/ci.yml`
- `infra/docker-compose.dev.yml` (Postgres + control plane + 1 Firecracker VM stub)
- Go type codegen from Zod schemas via `tygo` or hand-written to `control-plane/internal/apitypes/`

**Consumes**: nothing.

**Exit**: running `pnpm install && pnpm build` at repo root succeeds; `docker compose -f infra/docker-compose.dev.yml up` brings Postgres + control plane up and `/healthz` returns 200.

### Agent-Spine

**Scope**: `claude-wrap` binary, predicate eval, lesson content, fixture corpus, learner-bot.

**Owns**:
- `vm-image/claude-wrap/` (Go binary parsing `claude --output-format stream-json`)
- `guest-agent/cmd/learn-guest-agent/main.go` (new `watchClaudeWrap`, `watchFiles`, `watchFileContents`)
- `control-plane/internal/lesson/` (predicate eval, emits `step_satisfied`)
- `lessons/*.{es,en}.yml` (update 7 modules to new schema: `role_variants` for 8 departments, `expected_response_seconds`, `voice_first`, `requires_camera`; align predicates)
- `vm-image/sandbox/empresa-prueba/01-personas/` (Marta + 5 more per department, bilingual via `empresa-prueba.en/`)
- `tests/fixtures/claude/*.json` (corpus per lesson step)
- `tests/learner-bot/` (Go or TS harness)
- `vm-image/learn-shell.sh` (swap `claude` for `claude-wrap`)

**Consumes**: `shared/api/events.ts` (event types), `shared/lessons/schema.ts`.

**Exit**: running `go run ./tests/learner-bot -lesson m2` (or equivalent) against a real Claude produces every `claude_tool_call` in the fixture corpus and hits `step_satisfied` for all 4 steps of m2.

### Agent-API

**Scope**: control plane HTTP + WS + Postgres + auth + transcript + warm pool.

**Owns**:
- `control-plane/internal/api/` (handlers per section 5)
- `control-plane/internal/session/{manager,transcript,warmpool,heartbeat}.go`
- `control-plane/internal/identity/` (UUID minting, cookie signing)
- `control-plane/internal/auth/` (magic-link mint/consume, session tokens)
- `control-plane/internal/mail/resend.go` (Resend client)
- `control-plane/internal/db/migrations/*.sql`
- `control-plane/internal/db/queries/*.sql` (sqlc)
- `control-plane/cmd/learn-cp/main.go` (wiring)
- Server-side validators using Zod-generated Go types from Agent-Tooling

**Consumes**: `shared/api/schemas.ts`, `shared/api/events.ts`, `shared/api/errors.ts`.

**Exit**: full curl suite in `tests/api.sh` passes end-to-end: mint identity, request magic-link (email actually lands in `ops@example.com`), claim, GET /me, POST /sessions (warm pool serves in <500ms), connect WS, GET heartbeat, POST prompt with idempotency key, DELETE session. Postgres state verified via psql.

### Agent-Gate

**Scope**: Jailer + nftables + Platform proxy + Doppler.

**Owns**:
- `infra/jailer/` (Jailer wrapper scripts, per-VM seccomp + chroot + cgroups)
- `control-plane/internal/vm/jailer.go` (wrap Firecracker creation)
- `infra/nftables/learn.rules` (egress allowlist: `api.anthropic.com` + apt mirrors + proxy)
- `infra/systemd/{firecracker-jailer,claude-proxy}.service`
- `proxy/` (full Go binary: listens on `:8443`, forwards to `api.anthropic.com`, per-session quota, audit log to Postgres)
- Doppler setup docs + `doppler.yaml` at repo root
- `infra/bootstrap.sh` updates (idempotent)

**Consumes**: `shared/api/errors.ts` (for proxy error responses).

**Constraints**: Work against `infra/docker-compose.dev.yml` stack. Do NOT touch the live host (`learn-01` <host-ip>) during the sprint. Production cutover is a post-merge task.

**Exit**: in docker-compose.dev stack, a VM cannot reach `google.com`, a VM with exhausted quota receives 429 from the Platform proxy, all secrets loaded via `doppler run -- ./learn-cp`.

### Agent-Web

**Scope**: `apps/web/` learner surface. Three-phase arc (Fundamentos, Terminal guiada, Autonomia), xterm.js terminal plus wizard sidebar, anonymous identity flow, progress UI, telemetry hooks (t-to-first-wow, phase completion, D3/D7/D14 retention).

**Owns**:
- `apps/web/app/**` (App Router routes)
- `apps/web/components/**`
- `apps/web/lib/api.ts` (typed client from `shared/api`)
- `apps/web/lib/ws.ts` (wizard WS + PTY WS clients with reconnect)
- `apps/web/lib/xterm.tsx` (terminal wiring to PTY WS)
- `apps/web/lib/track.ts` (PostHog wrapper)

**Consumes**: `shared/api/schemas`, `shared/api/events`, `shared/tokens`, `shared/voice`, `shared/lessons`.

**Exit**: from a browser against the local dev stack, the learner completes `hello-claude.yml` end to end: enters anonymously, phase 1 concept screens resolve, phase 2 loads a real VM, a scaffolded prompt is sent to `claude-wrap`, tool-call cards stream in, `step_satisfied` arrives, the wizard advances, the session closes cleanly. Then the same flow against one converted lesson from the `learn-landing` 15-level catalog (candidate TBD in Fase 1 kickoff).

### Agent-Proof (out of scope for this monorepo)

The marketing/proof/certificate web lives in a separate repo (`learn-landing`) on its own VPS. Landing and app are decoupled by design: no shared deployment, no shared domain. The landing repo consumes the public control-plane endpoints (`GET /p/:uuid/public`, `GET /certificate/:id/public`) and imports `@learn/shared-api` + `@learn/shared-tokens` + `@learn/shared-voice` for contract and brand parity.

## 9. Conventions

- **Branches**: one branch per agent, named as in task descriptions (`contracts/scaffold`, `spine/predicates`, `api/endpoints`, `gate/security`, `web/proof`).
- **Commits**: imperative, <=72 chars first line, body wraps at 80. Reference issue IDs (#11, #12, #13, #14, #15) when a commit closes part of an issue.
- **PRs**: none during the 2-day sprint. Merge is manual at the end. Each agent pushes their branch, leader (me) merges.
- **Secrets**: never committed. Use Doppler (post Agent-Gate setup) or `.env.local` (git-ignored) for dev. No secret ever in `infra/bootstrap.sh` or systemd units.
- **Code comments**: minimal. Only for non-obvious "why". No "fixed X on date Y" comments; use git log.
- **Em dashes**: forbidden per root CLAUDE.md. Use comma or period.
- **Emojis**: forbidden unless user explicitly requests.
- **Language**: code identifiers English; user-facing strings through `shared/voice`. Comments Spanish or English, pick one per file.

## 10. How to ask a question

If a contract above is ambiguous:

1. Stop the work that depends on the ambiguity.
2. `gh issue create --repo mikelm20/learn-platform --label contracts-question --title "..." --body "..."`.
3. Continue on unrelated scope until the issue is resolved.

Do not invent. Do not guess. Do not "pick something reasonable". The cost of waiting 30 minutes for a contract answer is smaller than the cost of merging two incompatible interpretations.

---

End of contracts v1.
