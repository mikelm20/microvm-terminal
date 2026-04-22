import { z } from "zod";

const BaseEvent = z.object({
  ts: z.string(),
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
  port: z.number().int(),
});

export const ToolNameSchema = z.enum([
  "Read",
  "Write",
  "Edit",
  "Bash",
  "Glob",
  "Grep",
  "Task",
  "WebFetch",
  "WebSearch",
  "SlashCommand",
  "Other",
]);
export type ToolName = z.infer<typeof ToolNameSchema>;

export const ClaudePromptSentEvent = BaseEvent.extend({
  type: z.literal("claude_prompt_sent"),
  text: z.string(),
  length: z.number().int(),
  locale_detected: z.string().optional(),
});

export const ClaudeToolCallEvent = BaseEvent.extend({
  type: z.literal("claude_tool_call"),
  tool: ToolNameSchema,
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
  summary: z.string().optional(),
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
  duration_ms: z.number().int().optional(),
  turn_id: z.string(),
});

export const ClaudeBusyEvent = BaseEvent.extend({
  type: z.literal("claude_busy"),
  busy: z.boolean(),
});

export const ClaudeThinkingEvent = BaseEvent.extend({
  type: z.literal("claude_thinking"),
  chars_so_far: z.number().int(),
});

export const ClaudeTokenStreamedEvent = BaseEvent.extend({
  type: z.literal("claude_token_streamed"),
  turn_id: z.string(),
  delta: z.string(),
  total_chars: z.number().int(),
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
  evidence: z.unknown(),
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

// files_snapshot: emitted by the Capstone builder flow after Claude writes
// the mini-app files and the guest-agent responds to a list request. Lets
// the frontend show a real "Explore" sidebar so the learner sees actual code,
// not a black box.
export const FilesSnapshotEvent = BaseEvent.extend({
  type: z.literal("files_snapshot"),
  root: z.string(),
  files: z.array(z.object({
    path: z.string(),
    content: z.string(),
    size_bytes: z.number().int(),
  })),
});

export const WsEvent = z.discriminatedUnion("type", [
  SessionStartedEvent,
  VmBootingEvent,
  VmReadyEvent,
  AgentOnlineEvent,
  ProcessStartedEvent,
  PortListeningEvent,
  ClaudePromptSentEvent,
  ClaudeToolCallEvent,
  ClaudeToolResultEvent,
  ClaudeSlashCommandEvent,
  ClaudeMessageEvent,
  ClaudeBusyEvent,
  ClaudeThinkingEvent,
  ClaudeTokenStreamedEvent,
  FileExistsEvent,
  FileContentsRegexEvent,
  StepSatisfiedEvent,
  SessionErrorEvent,
  SessionClosedEvent,
  FilesSnapshotEvent,
]);
export type WsEvent = z.infer<typeof WsEvent>;
