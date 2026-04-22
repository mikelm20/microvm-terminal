import { z } from "zod";

export const LangSchema = z.enum(["es", "en"]);
export type Lang = z.infer<typeof LangSchema>;

export const DepartmentSchema = z.enum([
  "ventas",
  "marketing",
  "tecnologia",
  "producto",
  "finanzas",
  "rrhh",
  "legal",
  "estrategia",
]);
export type Department = z.infer<typeof DepartmentSchema>;

// identity
export const MintIdentityRequest = z.object({
  uuid: z.string().uuid().optional(),
  device_hint: z.string().optional(),
  lang: LangSchema,
});
export type MintIdentityRequest = z.infer<typeof MintIdentityRequest>;

export const MintIdentityResponse = z.object({
  uuid: z.string().uuid(),
  cookie: z.string(),
});
export type MintIdentityResponse = z.infer<typeof MintIdentityResponse>;

// auth
export const RequestMagicLinkRequest = z.object({
  email: z.string().email(),
  anonymous_uuid: z.string().uuid().optional(),
  lang: LangSchema,
});
export type RequestMagicLinkRequest = z.infer<typeof RequestMagicLinkRequest>;

export const RequestMagicLinkResponse = z.object({
  ok: z.literal(true),
  expires_in_seconds: z.number().int(),
});
export type RequestMagicLinkResponse = z.infer<typeof RequestMagicLinkResponse>;

export const ClaimAccountRequest = z.object({
  magic_link_token: z.string(),
  anonymous_uuid: z.string().uuid().optional(),
});
export type ClaimAccountRequest = z.infer<typeof ClaimAccountRequest>;

export const ClaimAccountResponse = z.object({
  uuid: z.string().uuid(),
  email: z.string().email(),
  session_token: z.string(),
});
export type ClaimAccountResponse = z.infer<typeof ClaimAccountResponse>;

// me
export const MeResponse = z.object({
  uuid: z.string().uuid(),
  email: z.string().email().nullable(),
  lang: LangSchema,
  department: DepartmentSchema.nullable(),
  name: z.string().nullable(),
  streak_days: z.number().int(),
  total_xp: z.number().int(),
  grace_tokens: z.number().int(),
  haptics_enabled: z.boolean(),
  push_enabled: z.boolean(),
  created_at: z.string(),
  claimed_at: z.string().nullable(),
});
export type MeResponse = z.infer<typeof MeResponse>;

export const PatchMeRequest = z.object({
  lang: LangSchema.optional(),
  department: DepartmentSchema.optional(),
  name: z.string().max(64).optional(),
  haptics_enabled: z.boolean().optional(),
  push_enabled: z.boolean().optional(),
});
export type PatchMeRequest = z.infer<typeof PatchMeRequest>;

export const PatchMeResponse = MeResponse;
export type PatchMeResponse = z.infer<typeof PatchMeResponse>;

// progress
export const ModuleProgressSchema = z.object({
  completed_steps: z.array(z.string()),
  paused_at: z.string().nullable(),
  xp_earned: z.number().int(),
});
export type ModuleProgress = z.infer<typeof ModuleProgressSchema>;

export const ProgressStateSchema = z.object({
  modules: z.record(z.string(), ModuleProgressSchema),
  streak_days: z.number().int(),
  grace_tokens: z.number().int(),
  last_visited: z.string(),
  seen_coachmarks: z.array(z.string()),
  updated_at: z.string(),
});
export type ProgressState = z.infer<typeof ProgressStateSchema>;

export const ProgressSyncRequest = z.object({
  uuid: z.string().uuid(),
  state: ProgressStateSchema,
});
export type ProgressSyncRequest = z.infer<typeof ProgressSyncRequest>;

export const ProgressSyncResponse = z.object({
  state: ProgressStateSchema,
  server_updated_at: z.string(),
});
export type ProgressSyncResponse = z.infer<typeof ProgressSyncResponse>;

// lessons
export const LessonSummarySchema = z.object({
  id: z.string(),
  module_number: z.number().int(),
  language: LangSchema,
  title: z.string(),
  subtitle: z.string().optional(),
  estimated_minutes: z.number().int().optional(),
});
export type LessonSummary = z.infer<typeof LessonSummarySchema>;

export const ListLessonsResponse = z.object({
  lessons: z.array(LessonSummarySchema),
});
export type ListLessonsResponse = z.infer<typeof ListLessonsResponse>;

// sessions
export const CreateSessionRequest = z.object({
  lesson_id: z.string(),
  lang: LangSchema,
  warm: z.boolean().optional(),
});
export type CreateSessionRequest = z.infer<typeof CreateSessionRequest>;

export const CreateSessionResponse = z.object({
  session_id: z.string().uuid(),
  vm_ip: z.string(),
  pty_ws_url: z.string().url(),
  wizard_ws_url: z.string().url(),
  preview_url_template: z.string(),
});
export type CreateSessionResponse = z.infer<typeof CreateSessionResponse>;

export const HeartbeatResponse = z.object({
  session_id: z.string().uuid(),
  vm_ip: z.string(),
  alive: z.boolean(),
  claude_busy: z.boolean(),
  last_event_at: z.string(),
  age_seconds: z.number().int(),
});
export type HeartbeatResponse = z.infer<typeof HeartbeatResponse>;

export const TranscriptResponse = z.object({
  session_id: z.string().uuid(),
  events: z.array(z.unknown()),
});
export type TranscriptResponse = z.infer<typeof TranscriptResponse>;

export const SubmitPromptRequest = z.object({
  text: z.string().min(1).max(4096),
  client_ts: z.string(),
});
export type SubmitPromptRequest = z.infer<typeof SubmitPromptRequest>;

export const SubmitPromptResponse = z.object({
  ok: z.literal(true),
  turn_id: z.string(),
});
export type SubmitPromptResponse = z.infer<typeof SubmitPromptResponse>;

export const AttachImageResponse = z.object({
  inbox_path: z.string(),
});
export type AttachImageResponse = z.infer<typeof AttachImageResponse>;

// public (no auth) shapes consumed by the separate marketing/proof web (learn-landing)
export const PublicProfileResponse = z.object({
  uuid: z.string().uuid(),
  name: z.string().nullable(),
  department: DepartmentSchema.nullable(),
  lang: LangSchema,
  streak_days: z.number().int(),
  total_xp: z.number().int(),
  modules_completed: z.array(z.object({
    lesson_id: z.string(),
    title: z.string(),
    completed_at: z.string(),
    takeaways: z.array(z.string()),
  })),
});
export type PublicProfileResponse = z.infer<typeof PublicProfileResponse>;

export const PublicCertificateResponse = z.object({
  id: z.string(),
  uuid: z.string().uuid(),
  name: z.string().nullable(),
  department: DepartmentSchema.nullable(),
  lang: LangSchema,
  completed_at: z.string(),
  signature: z.string(),
  modules: z.array(z.string()),
});
export type PublicCertificateResponse = z.infer<typeof PublicCertificateResponse>;

// capstone (F2 Lab closing flow)
// Validate: short-lived Platform validator VM reads the learner's brief, replies
// with strict JSON {ok, reason?}. Endpoint is POST /capstone/validate.
export const CapstoneValidateRequest = z.object({
  prompt: z.string().min(1).max(500),
  lang: LangSchema.optional(),
  topic_hint: z.string().optional(),
});
export type CapstoneValidateRequest = z.infer<typeof CapstoneValidateRequest>;

export const CapstoneValidateResponse = z.object({
  ok: z.boolean(),
  reason: z.string().optional(),
});
export type CapstoneValidateResponse = z.infer<typeof CapstoneValidateResponse>;

// Build: long-lived builder VM receives the injection-wrapped brief and
// Claude writes files + launches python3 -m http.server 3000. Endpoint is
// POST /capstone/build. Response carries enough to mount the wizard WS +
// preview iframe on the client.
export const CapstoneBuildRequest = z.object({
  prompt: z.string().min(1).max(500),
  lang: LangSchema.optional(),
});
export type CapstoneBuildRequest = z.infer<typeof CapstoneBuildRequest>;

export const CapstoneBuildResponse = z.object({
  session_id: z.string().uuid(),
  vm_ip: z.string(),
  wizard_ws_url: z.string().url(),
  preview_url_template: z.string(),
  turn_id: z.string(),
});
export type CapstoneBuildResponse = z.infer<typeof CapstoneBuildResponse>;
