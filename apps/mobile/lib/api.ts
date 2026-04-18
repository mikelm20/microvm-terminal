import { z, type ZodSchema } from "zod";
import {
  CreateSessionRequest,
  CreateSessionResponse,
  HeartbeatResponse,
  MeResponse,
  PatchMeRequest,
  PatchMeResponse,
  RequestMagicLinkRequest,
  RequestMagicLinkResponse,
  ClaimAccountRequest,
  ClaimAccountResponse,
  SubmitPromptRequest,
  SubmitPromptResponse,
  TranscriptResponse,
  AttachImageResponse,
} from "@learn/shared-api/schemas";
import { ApiError as ApiErrorSchema, type ApiError as ApiErrorShape } from "@learn/shared-api/errors";

/**
 * Typed fetch wrapper for the control plane. Every response goes through
 * Zod safeParse. Every non-2xx is translated to a typed ApiError.
 */

export const API_BASE =
  process.env.EXPO_PUBLIC_API_BASE ?? "http://localhost:8080";

export class ApiError extends Error {
  readonly code: ApiErrorShape["code"];
  readonly request_id: string;
  readonly retry_after_seconds?: number;
  readonly queue_position?: number;
  readonly status: number;

  constructor(err: ApiErrorShape, status: number) {
    super(err.message);
    this.code = err.code;
    this.request_id = err.request_id;
    this.retry_after_seconds = err.retry_after_seconds;
    this.queue_position = err.queue_position;
    this.status = status;
  }
}

export class TransportError extends Error {
  readonly status: number;
  constructor(message: string, status = 0) {
    super(message);
    this.status = status;
  }
}

export class ValidationError extends Error {
  readonly issues: z.ZodIssue[];
  constructor(issues: z.ZodIssue[]) {
    super("Response did not match schema");
    this.issues = issues;
  }
}

interface RequestOptions<B> {
  method?: "GET" | "POST" | "PATCH" | "PUT" | "DELETE";
  body?: B;
  headers?: Record<string, string>;
  idempotencyKey?: string;
  signal?: AbortSignal;
}

async function request<B, R>(
  path: string,
  responseSchema: ZodSchema<R>,
  opts: RequestOptions<B> = {},
): Promise<R> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Accept: "application/json",
    ...opts.headers,
  };
  if (opts.idempotencyKey) headers["Idempotency-Key"] = opts.idempotencyKey;

  const url = path.startsWith("http") ? path : `${API_BASE}${path}`;

  let res: Response;
  try {
    res = await fetch(url, {
      method: opts.method ?? "GET",
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      credentials: "include",
      signal: opts.signal,
    });
  } catch (e) {
    throw new TransportError(e instanceof Error ? e.message : "network error");
  }

  const text = await res.text();
  const json = text.length > 0 ? safeJson(text) : {};

  if (!res.ok) {
    const parsed = ApiErrorSchema.safeParse(json);
    if (parsed.success) throw new ApiError(parsed.data, res.status);
    throw new TransportError(`HTTP ${res.status}`, res.status);
  }

  const parsed = responseSchema.safeParse(json);
  if (!parsed.success) throw new ValidationError(parsed.error.issues);
  return parsed.data;
}

function safeJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return {};
  }
}

export const api = {
  async me(): Promise<z.infer<typeof MeResponse>> {
    return request("/me", MeResponse);
  },

  async createSession(
    input: z.infer<typeof CreateSessionRequest>,
  ): Promise<z.infer<typeof CreateSessionResponse>> {
    return request("/sessions", CreateSessionResponse, {
      method: "POST",
      body: input,
    });
  },

  async getHeartbeat(sessionId: string): Promise<z.infer<typeof HeartbeatResponse>> {
    return request(`/sessions/${sessionId}/heartbeat`, HeartbeatResponse);
  },

  async getTranscript(sessionId: string): Promise<z.infer<typeof TranscriptResponse>> {
    return request(`/sessions/${sessionId}/transcript`, TranscriptResponse);
  },

  async submitPrompt(
    sessionId: string,
    input: z.infer<typeof SubmitPromptRequest>,
    idempotencyKey: string,
  ): Promise<z.infer<typeof SubmitPromptResponse>> {
    return request(`/sessions/${sessionId}/prompt`, SubmitPromptResponse, {
      method: "POST",
      body: input,
      idempotencyKey,
    });
  },

  async attachImage(
    sessionId: string,
    file: { uri: string; name: string; mime: string },
  ): Promise<z.infer<typeof AttachImageResponse>> {
    const url = `${API_BASE}/sessions/${sessionId}/attach`;
    const form = new FormData();
    // React Native multipart shape
    form.append("image", {
      uri: file.uri,
      name: file.name,
      type: file.mime,
    } as unknown as Blob);

    let res: Response;
    try {
      res = await fetch(url, { method: "POST", body: form, credentials: "include" });
    } catch (e) {
      throw new TransportError(e instanceof Error ? e.message : "network error");
    }
    const text = await res.text();
    const json = text.length > 0 ? safeJson(text) : {};
    if (!res.ok) {
      const parsed = ApiErrorSchema.safeParse(json);
      if (parsed.success) throw new ApiError(parsed.data, res.status);
      throw new TransportError(`HTTP ${res.status}`, res.status);
    }
    const parsed = AttachImageResponse.safeParse(json);
    if (!parsed.success) throw new ValidationError(parsed.error.issues);
    return parsed.data;
  },

  async deleteSession(sessionId: string): Promise<void> {
    await request(`/sessions/${sessionId}`, z.object({ ok: z.literal(true) }), {
      method: "DELETE",
    });
  },
};

/** Build the websocket URL for a session's wizard channel. */
export function wizardWsUrl(sessionId: string): string {
  const base = API_BASE.replace(/^http/, "ws");
  return `${base}/sessions/${sessionId}/ws`;
}

/** Base URL for identity + progress flows that live outside the typed `api` object. */
export const apiBaseUrl = API_BASE;

export async function requestMagicLink(
  email: string,
  lang: "es" | "en",
  anonymousUuid?: string,
): Promise<z.infer<typeof RequestMagicLinkResponse>> {
  const body: z.infer<typeof RequestMagicLinkRequest> = {
    email,
    lang,
    ...(anonymousUuid ? { anonymous_uuid: anonymousUuid } : {}),
  };
  return request("/auth/magic-link", RequestMagicLinkResponse, { method: "POST", body });
}

export async function claimMagicLink(
  token: string,
  anonymousUuid?: string,
): Promise<z.infer<typeof ClaimAccountResponse>> {
  const body: z.infer<typeof ClaimAccountRequest> = {
    magic_link_token: token,
    ...(anonymousUuid ? { anonymous_uuid: anonymousUuid } : {}),
  };
  return request("/auth/claim", ClaimAccountResponse, { method: "POST", body });
}

export async function patchMe(
  input: z.infer<typeof PatchMeRequest>,
): Promise<z.infer<typeof PatchMeResponse>> {
  return request("/me", PatchMeResponse, { method: "PATCH", body: input });
}

export async function setDepartment(
  department: z.infer<typeof PatchMeRequest>["department"],
): Promise<z.infer<typeof PatchMeResponse>> {
  return patchMe({ department });
}
