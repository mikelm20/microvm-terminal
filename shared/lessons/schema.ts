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

export const PredicateSchema = z.discriminatedUnion("type", [
  z.object({ type: z.literal("agent_online") }),
  z.object({
    type: z.literal("process_started"),
    match: z.object({ name: z.string() }),
  }),
  z.object({
    type: z.literal("port_listening"),
    match: z.object({ port: z.number().optional() }).optional(),
  }),
  z.object({
    type: z.literal("claude_prompt_sent"),
    match_regex: z.string().optional(),
  }),
  z.object({
    type: z.literal("claude_tool_call"),
    match: z.object({
      tool: z.string(),
      path_regex: z.string().optional(),
      command_regex: z.string().optional(),
      subagent_type: z.string().optional(),
    }),
  }),
  z.object({
    type: z.literal("claude_slash_command"),
    match: z.object({ command: z.string() }),
  }),
  z.object({
    type: z.literal("file_exists"),
    match: z.object({ path: z.string() }),
  }),
  z.object({
    type: z.literal("file_contents_regex"),
    match: z.object({ path: z.string(), regex: z.string() }),
  }),
]);
export type Predicate = z.infer<typeof PredicateSchema>;

export const RoleVariantSchema = z.object({
  body: z.string(),
  prompt: z.string().optional(),
  hint: z.string().optional(),
});
export type RoleVariant = z.infer<typeof RoleVariantSchema>;

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
  expected_response_seconds: z.number().optional(),
  voice_first: z.boolean().optional(),
  requires_camera: z.boolean().optional(),
  role_variants: z.record(DepartmentSchema, RoleVariantSchema).optional(),
  // Per-step system prompt injected via claude-wrap's -system-prompt-file.
  // When present, claude is primed with this text so a specific semantic
  // input deterministically produces the expected teaching output. Empty
  // or missing means free-form Claude. Used first by F2 Lab substeps where
  // the canonical answer must be stable across runs.
  system_prompt: z.string().optional(),
});
export type LessonStep = z.infer<typeof LessonStepSchema>;

export const LessonSchema = z.object({
  id: z.string(),
  module_number: z.number(),
  language: LangSchema,
  title: z.string(),
  subtitle: z.string().optional(),
  description: z.string().optional(),
  estimated_minutes: z.number().optional(),
  sandbox: z.string().optional(),
  mental_model: z.string().optional(),
  preconditions: z
    .object({
      working_directory: z.string().optional(),
      claude_running: z.boolean().optional(),
      files_present: z.array(z.string()).optional(),
    })
    .optional(),
  steps: z.array(LessonStepSchema),
  on_complete: z
    .object({
      total_xp: z.number().optional(),
      takeaways: z.array(z.string()).optional(),
      next_module: z.string().nullable().optional(),
      course_complete: z.boolean().optional(),
      trigger_signup: z.boolean().optional(),
      milestone_screen: z.string().optional(),
    })
    .optional(),
});
export type Lesson = z.infer<typeof LessonSchema>;
