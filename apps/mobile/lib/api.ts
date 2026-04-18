// PLACEHOLDER. Agent-Convo owns this file. Until their branch is merged,
// agent-path only needs the handful of calls referenced by the screens it
// ships. The real client mirrors @learn/shared-api and is a swap-file
// replacement. Do not grow this stub: if a new endpoint is needed, wait
// for the merged client.

import Constants from "expo-constants";
import {
  RequestMagicLinkRequest,
  RequestMagicLinkResponse,
  ClaimAccountRequest,
  ClaimAccountResponse,
  PatchMeRequest,
  MeResponse,
  type Lang,
  type Department,
} from "@learn/shared-api/schemas";
import { getIdentity, saveCookie, saveClaim } from "./identity";

export function apiBaseUrl(): string {
  const fromConfig =
    (Constants.expoConfig?.extra?.apiBaseUrl as string | undefined) ?? undefined;
  return fromConfig ?? "http://localhost:8080";
}

async function authHeader(): Promise<Record<string, string>> {
  const id = await getIdentity();
  return id.cookie ? { Cookie: id.cookie } : {};
}

export async function requestMagicLink(email: string, lang: Lang): Promise<void> {
  const id = await getIdentity();
  const body = RequestMagicLinkRequest.parse({
    email,
    anonymous_uuid: id.uuid,
    lang,
  });
  const res = await fetch(`${apiBaseUrl()}/auth/magic-link`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...(await authHeader()) },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(`magic-link failed: ${res.status}`);
  }
  RequestMagicLinkResponse.parse(await res.json());
}

export async function claimMagicLink(token: string): Promise<ClaimAccountResponse> {
  const id = await getIdentity();
  const body = ClaimAccountRequest.parse({
    magic_link_token: token,
    anonymous_uuid: id.uuid,
  });
  const res = await fetch(`${apiBaseUrl()}/auth/claim`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(`claim failed: ${res.status}`);
  }
  const parsed = ClaimAccountResponse.parse(await res.json());
  await saveClaim(parsed.email, parsed.uuid);
  const setCookie = res.headers.get("set-cookie");
  if (setCookie) await saveCookie(setCookie);
  return parsed;
}

export async function patchMe(patch: PatchMeRequest): Promise<MeResponse | null> {
  const body = PatchMeRequest.parse(patch);
  try {
    const res = await fetch(`${apiBaseUrl()}/me`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", ...(await authHeader()) },
      body: JSON.stringify(body),
    });
    if (!res.ok) return null;
    return MeResponse.parse(await res.json());
  } catch {
    return null;
  }
}

export async function setDepartment(department: Department): Promise<void> {
  await patchMe({ department });
}
