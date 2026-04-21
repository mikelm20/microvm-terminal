import type { z } from "zod";
import {
  CreateSessionRequest,
  CreateSessionResponse,
  HeartbeatResponse,
  ListLessonsResponse,
  MintIdentityRequest,
  MintIdentityResponse,
} from "@learn/shared-api/schemas";
import { ApiError } from "@learn/shared-api/errors";

export const DEFAULT_API_BASE_URL = "http://localhost:8080";

export function apiBaseUrl(): string {
  return (
    process.env.NEXT_PUBLIC_API_BASE_URL?.trim() || DEFAULT_API_BASE_URL
  );
}

export class ApiClientError extends Error {
  readonly status: number;
  readonly body: ApiError | null;

  constructor(status: number, message: string, body: ApiError | null) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

type FetchOptions = {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
  signal?: AbortSignal;
};

async function request<T>(
  path: string,
  schema: z.ZodType<T>,
  options: FetchOptions = {},
): Promise<T> {
  const { method = "GET", body, headers = {}, signal } = options;
  const init: RequestInit = {
    method,
    headers: {
      "Content-Type": "application/json",
      ...headers,
    },
    signal,
    credentials: "include",
  };
  if (body !== undefined) {
    init.body = JSON.stringify(body);
  }

  const res = await fetch(`${apiBaseUrl()}${path}`, init);
  const text = await res.text();
  const parsed: unknown = text.length > 0 ? JSON.parse(text) : null;

  if (!res.ok) {
    const err = ApiError.safeParse(parsed);
    throw new ApiClientError(
      res.status,
      err.success ? err.data.message : res.statusText,
      err.success ? err.data : null,
    );
  }

  return schema.parse(parsed);
}

export async function mintIdentity(
  input: MintIdentityRequest,
  opts: FetchOptions = {},
): Promise<MintIdentityResponse> {
  return request("/identity", MintIdentityResponse, {
    ...opts,
    method: "POST",
    body: MintIdentityRequest.parse(input),
  });
}

export async function listLessons(
  opts: FetchOptions = {},
): Promise<ListLessonsResponse> {
  return request("/lessons", ListLessonsResponse, opts);
}

export async function createSession(
  input: CreateSessionRequest,
  opts: FetchOptions = {},
): Promise<CreateSessionResponse> {
  return request("/sessions", CreateSessionResponse, {
    ...opts,
    method: "POST",
    body: CreateSessionRequest.parse(input),
  });
}

export async function heartbeat(
  sessionId: string,
  opts: FetchOptions = {},
): Promise<HeartbeatResponse> {
  return request(
    `/sessions/${encodeURIComponent(sessionId)}/heartbeat`,
    HeartbeatResponse,
    opts,
  );
}
