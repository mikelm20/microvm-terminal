#!/usr/bin/env node
// Generate control-plane/internal/apitypes/apitypes.go from the Zod schemas
// in shared/api/{schemas,events,errors}.ts.
//
// Strategy: import the Zod schemas from TS source (via tsx), pass each through
// zod-to-json-schema, then walk the resulting JSON Schema to emit Go structs.
// Output is committed so Go agents do not need the Node toolchain.
//
// Run: `pnpm codegen:api` (runs via tsx).

import { writeFileSync, mkdirSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import { dirname, join } from "node:path";
import { spawnSync } from "node:child_process";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const outDir = join(repoRoot, "control-plane", "internal", "apitypes");
const outFile = join(outDir, "apitypes.go");

const { zodToJsonSchema } = await import("zod-to-json-schema");
const schemas = await import(pathToFileURL(join(here, "schemas.ts")).href);
const events = await import(pathToFileURL(join(here, "events.ts")).href);
const errors = await import(pathToFileURL(join(here, "errors.ts")).href);

const apiExports = [
  ["MintIdentityRequest", "MintIdentityRequest"],
  ["MintIdentityResponse", "MintIdentityResponse"],
  ["RequestMagicLinkRequest", "RequestMagicLinkRequest"],
  ["RequestMagicLinkResponse", "RequestMagicLinkResponse"],
  ["ClaimAccountRequest", "ClaimAccountRequest"],
  ["ClaimAccountResponse", "ClaimAccountResponse"],
  ["MeResponse", "MeResponse"],
  ["PatchMeRequest", "PatchMeRequest"],
  ["PatchMeResponse", "PatchMeResponse"],
  ["ModuleProgressSchema", "ModuleProgress"],
  ["ProgressStateSchema", "ProgressState"],
  ["ProgressSyncRequest", "ProgressSyncRequest"],
  ["ProgressSyncResponse", "ProgressSyncResponse"],
  ["LessonSummarySchema", "LessonSummary"],
  ["ListLessonsResponse", "ListLessonsResponse"],
  ["CreateSessionRequest", "CreateSessionRequest"],
  ["CreateSessionResponse", "CreateSessionResponse"],
  ["HeartbeatResponse", "HeartbeatResponse"],
  ["TranscriptResponse", "TranscriptResponse"],
  ["SubmitPromptRequest", "SubmitPromptRequest"],
  ["SubmitPromptResponse", "SubmitPromptResponse"],
  ["AttachImageResponse", "AttachImageResponse"],
  ["PublicProfileResponse", "PublicProfileResponse"],
  ["PublicCertificateResponse", "PublicCertificateResponse"],
  ["CapstoneValidateRequest", "CapstoneValidateRequest"],
  ["CapstoneValidateResponse", "CapstoneValidateResponse"],
  ["CapstoneBuildRequest", "CapstoneBuildRequest"],
  ["CapstoneBuildResponse", "CapstoneBuildResponse"],
];

const eventExports = [
  ["SessionStartedEvent", "SessionStartedEvent"],
  ["VmBootingEvent", "VmBootingEvent"],
  ["VmReadyEvent", "VmReadyEvent"],
  ["AgentOnlineEvent", "AgentOnlineEvent"],
  ["ProcessStartedEvent", "ProcessStartedEvent"],
  ["PortListeningEvent", "PortListeningEvent"],
  ["ClaudePromptSentEvent", "ClaudePromptSentEvent"],
  ["ClaudeToolCallEvent", "ClaudeToolCallEvent"],
  ["ClaudeToolResultEvent", "ClaudeToolResultEvent"],
  ["ClaudeSlashCommandEvent", "ClaudeSlashCommandEvent"],
  ["ClaudeMessageEvent", "ClaudeMessageEvent"],
  ["ClaudeBusyEvent", "ClaudeBusyEvent"],
  ["ClaudeThinkingEvent", "ClaudeThinkingEvent"],
  ["ClaudeTokenStreamedEvent", "ClaudeTokenStreamedEvent"],
  ["FileExistsEvent", "FileExistsEvent"],
  ["FileContentsRegexEvent", "FileContentsRegexEvent"],
  ["StepSatisfiedEvent", "StepSatisfiedEvent"],
  ["SessionErrorEvent", "SessionErrorEvent"],
  ["SessionClosedEvent", "SessionClosedEvent"],
  // FilesSnapshotFile is listed before FilesSnapshotEvent so the nested array
  // item reuses the named struct instead of a synthesized *FilesItem name.
  ["FilesSnapshotFile", "FilesSnapshotFile"],
  ["FilesSnapshotEvent", "FilesSnapshotEvent"],
];

const errorExports = [["ApiError", "ApiError"]];

// Preferred top-level struct names for nested objects keyed by shape JSON.
// Populated as we register top-level structs so nested uses reuse them.
const shapeToName = new Map();
const structs = []; // in emission order
const emitted = new Set(); // struct names already emitted

function pascal(s) {
  const acronyms = new Set(["id", "uuid", "url", "ip", "ws", "api", "vm", "pty", "xp", "ts"]);
  return s
    .split(/[_-]/)
    .map((p) => {
      if (p.length === 0) return p;
      if (acronyms.has(p.toLowerCase())) return p.toUpperCase();
      return p[0].toUpperCase() + p.slice(1);
    })
    .join("");
}

// Canonicalize schema for dedup: sort object keys, drop metadata-only fields
// that zod-to-json-schema sometimes inlines (description, $schema, additionalProperties
// when it is an empty boolean-esque value). This keeps structurally-equal shapes
// collapsing to the same key even if they are declared at top-level vs inlined.
function canonical(x) {
  if (Array.isArray(x)) return x.map(canonical);
  if (x && typeof x === "object") {
    const out = {};
    const skip = new Set(["$schema", "description", "title"]);
    const keys = Object.keys(x).filter((k) => !skip.has(k)).sort();
    for (const k of keys) out[k] = canonical(x[k]);
    return out;
  }
  return x;
}

function schemaKey(s) {
  return JSON.stringify(canonical(s));
}

// Normalize nullable: returns { schema, nullable }.
function unwrapNullable(s) {
  if (!s) return { schema: s, nullable: false };
  if (Array.isArray(s.type) && s.type.includes("null")) {
    const nonNull = s.type.filter((t) => t !== "null");
    if (nonNull.length === 1) {
      return { schema: { ...s, type: nonNull[0] }, nullable: true };
    }
  }
  const variants = s.anyOf ?? s.oneOf;
  if (Array.isArray(variants)) {
    const nullVariant = variants.find((v) => v && v.type === "null");
    const nonNull = variants.filter((v) => !(v && v.type === "null"));
    if (nullVariant && nonNull.length === 1) {
      return { schema: nonNull[0], nullable: true };
    }
  }
  return { schema: s, nullable: false };
}

function goScalar(schema) {
  if (Array.isArray(schema.enum) && schema.enum.every((e) => typeof e === "string")) {
    return "string";
  }
  if (schema.const !== undefined) {
    if (typeof schema.const === "string") return "string";
    if (typeof schema.const === "number") return "float64";
    if (typeof schema.const === "boolean") return "bool";
  }
  switch (schema.type) {
    case "string":
      return "string";
    case "integer":
      return "int";
    case "number":
      return "float64";
    case "boolean":
      return "bool";
    default:
      return null;
  }
}

function ensureStruct(goName, schema, needsJsonRef) {
  if (!shapeToName.has(schemaKey(schema))) {
    shapeToName.set(schemaKey(schema), goName);
  }
  if (emitted.has(goName)) return goName;
  emitted.add(goName);
  emitStruct(goName, schema, needsJsonRef);
  return goName;
}

function mapFieldType(fieldSchema, suggestedName, needsJsonRef) {
  const { schema, nullable } = unwrapNullable(fieldSchema);
  if (!schema) return { goType: "json.RawMessage", nullable: false };

  const scalar = goScalar(schema);
  if (scalar) {
    return { goType: scalar, nullable };
  }

  if (schema.type === "array") {
    const inner = mapFieldType(schema.items, suggestedName + "Item", needsJsonRef);
    const innerType =
      inner.nullable && !inner.goType.startsWith("[]") && !inner.goType.startsWith("map[")
        ? `*${inner.goType}`
        : inner.goType;
    return { goType: `[]${innerType}`, nullable };
  }

  if (schema.type === "object") {
    // Record: additionalProperties + no named properties.
    if (
      schema.additionalProperties &&
      (!schema.properties || Object.keys(schema.properties).length === 0)
    ) {
      const inner = mapFieldType(
        schema.additionalProperties,
        suggestedName + "Value",
        needsJsonRef,
      );
      const innerType =
        inner.nullable && !inner.goType.startsWith("[]") && !inner.goType.startsWith("map[")
          ? `*${inner.goType}`
          : inner.goType;
      return { goType: `map[string]${innerType}`, nullable };
    }
    // Named nested object, dedupe by shape.
    if (schema.properties && Object.keys(schema.properties).length > 0) {
      const existing = shapeToName.get(schemaKey(schema));
      if (existing) return { goType: existing, nullable };
      const name = suggestedName;
      ensureStruct(name, schema, needsJsonRef);
      return { goType: name, nullable };
    }
    // Open object.
    needsJsonRef.used = true;
    return { goType: "json.RawMessage", nullable };
  }

  // anyOf / oneOf without a null variant (unions): fall through to RawMessage.
  needsJsonRef.used = true;
  return { goType: "json.RawMessage", nullable };
}

function emitStruct(goName, schema, needsJsonRef) {
  const required = new Set(schema.required ?? []);
  const props = schema.properties ?? {};
  const lines = [];
  lines.push(`type ${goName} struct {`);
  for (const [jsonName, propSchema] of Object.entries(props)) {
    const suggested = goName + pascal(jsonName);
    const { goType, nullable } = mapFieldType(propSchema, suggested, needsJsonRef);
    const optional = !required.has(jsonName);
    let finalType = goType;
    const isSliceOrMap = finalType.startsWith("[]") || finalType.startsWith("map[");
    const isRawMessage = finalType === "json.RawMessage";
    // Pointer when nullable or optional, and not already a nilable Go type.
    if ((nullable || optional) && !isSliceOrMap && !isRawMessage) {
      finalType = `*${finalType}`;
    }
    const omitempty = optional || nullable ? ",omitempty" : "";
    const fieldName = pascal(jsonName);
    const tag = "`" + `json:"${jsonName}${omitempty}"` + "`";
    lines.push(`\t${fieldName} ${finalType} ${tag}`);
  }
  lines.push("}");
  structs.push(lines.join("\n"));
}

function compile(exports, sourceModule) {
  const needsJsonRef = { used: false };
  for (const [exportName, goName] of exports) {
    const zodSchema = sourceModule[exportName];
    if (!zodSchema) throw new Error(`Missing export: ${exportName}`);
    let json = zodToJsonSchema(zodSchema, { $refStrategy: "none" });
    if (json.$ref && json.definitions) {
      const ref = json.$ref.replace(/^#\/definitions\//, "");
      json = json.definitions[ref];
    }
    // Pre-register the shape under the desired top-level name so nested uses
    // in other top-level schemas reuse it.
    shapeToName.set(schemaKey(json), goName);
  }
  for (const [exportName, goName] of exports) {
    const zodSchema = sourceModule[exportName];
    let json = zodToJsonSchema(zodSchema, { $refStrategy: "none" });
    if (json.$ref && json.definitions) {
      const ref = json.$ref.replace(/^#\/definitions\//, "");
      json = json.definitions[ref];
    }
    if (!emitted.has(goName)) {
      emitted.add(goName);
      emitStruct(goName, json, needsJsonRef);
    }
  }
  return needsJsonRef.used;
}

let usesJson = false;
usesJson = compile(apiExports, schemas) || usesJson;
usesJson = compile(errorExports, errors) || usesJson;
usesJson = compile(eventExports, events) || usesJson;

const wsEnvelope = `// WsEventEnvelope is the minimal shape needed to discriminate a WsEvent by
// type before full decode. WsEventType lists every known discriminator;
// mirrors the TS discriminated union in shared/api/events.ts.
type WsEventEnvelope struct {
\tType string \`json:"type"\`
\tTs   string \`json:"ts"\`
}

type WsEventType string

const (
\tWsEventSessionStarted     WsEventType = "session_started"
\tWsEventVmBooting          WsEventType = "vm_booting"
\tWsEventVmReady            WsEventType = "vm_ready"
\tWsEventAgentOnline        WsEventType = "agent_online"
\tWsEventProcessStarted     WsEventType = "process_started"
\tWsEventPortListening      WsEventType = "port_listening"
\tWsEventClaudePromptSent   WsEventType = "claude_prompt_sent"
\tWsEventClaudeToolCall     WsEventType = "claude_tool_call"
\tWsEventClaudeToolResult   WsEventType = "claude_tool_result"
\tWsEventClaudeSlashCommand WsEventType = "claude_slash_command"
\tWsEventClaudeMessage      WsEventType = "claude_message"
\tWsEventClaudeBusy         WsEventType = "claude_busy"
\tWsEventClaudeThinking     WsEventType = "claude_thinking"
\tWsEventClaudeTokenStream  WsEventType = "claude_token_streamed"
\tWsEventFileExists         WsEventType = "file_exists"
\tWsEventFileContentsRegex  WsEventType = "file_contents_regex"
\tWsEventStepSatisfied      WsEventType = "step_satisfied"
\tWsEventSessionError       WsEventType = "session_error"
\tWsEventSessionClosed      WsEventType = "session_closed"
\tWsEventFilesSnapshot      WsEventType = "files_snapshot"
)

// ApiErrorCode enumerates every known control-plane error code. Mirrors the
// ApiErrorCode enum in shared/api/errors.ts.
type ApiErrorCode string

const (
\tApiErrorCapacityFull      ApiErrorCode = "capacity_full"
\tApiErrorAuthRequired      ApiErrorCode = "auth_required"
\tApiErrorAuthInvalid       ApiErrorCode = "auth_invalid"
\tApiErrorMagicLinkExpired  ApiErrorCode = "magic_link_expired"
\tApiErrorVMLaunchFailed    ApiErrorCode = "vm_launch_failed"
\tApiErrorVMNotFound        ApiErrorCode = "vm_not_found"
\tApiErrorSessionReaped     ApiErrorCode = "session_reaped"
\tApiErrorBadRequest        ApiErrorCode = "bad_request"
\tApiErrorNotFound          ApiErrorCode = "not_found"
\tApiErrorRateLimited       ApiErrorCode = "rate_limited"
\tApiErrorInternal          ApiErrorCode = "internal"
)
`;

const importBlock = usesJson ? `import (\n\t"encoding/json"\n)\n\nvar _ = json.Marshal\n` : ``;

const header = `// Code generated by shared/api/codegen.mjs. DO NOT EDIT.
//
// Source: shared/api/{schemas,events,errors}.ts via zod-to-json-schema.
// Regenerate with: pnpm codegen:api (from repo root).
//
// JSON tags mirror the wire format exactly. Optional and nullable fields use
// pointer types with omitempty so (nil) round-trips as the field missing from
// the wire.

package apitypes

${importBlock}`;

mkdirSync(outDir, { recursive: true });
const output = header + "\n" + structs.join("\n\n") + "\n\n" + wsEnvelope + "\n";
writeFileSync(outFile, output, "utf8");

// Best-effort gofmt. Skipped silently if `go` is not on PATH.
const gofmt = spawnSync("gofmt", ["-w", outFile], { stdio: "inherit" });
if (gofmt.status !== 0 && gofmt.error) {
  console.warn(`gofmt not available, skipping (${gofmt.error.code}).`);
}

console.log(
  `Wrote ${outFile} (${structs.length} structs, json import: ${usesJson ? "yes" : "no"}).`,
);
