// Deep-link parsing + registration. Expo Router handles the routing itself
// (our URL path /auth/callback matches the file-route), this helper only
// exists to validate and extract the magic-link token, and to expose the
// prefix used by expo-auth-session when it needs one.

import * as Linking from "expo-linking";

export const SCHEME = "learn";

export type DeepLinkPayload =
  | { kind: "auth-callback"; token: string }
  | { kind: "unknown"; url: string };

export function parseDeepLink(url: string | null | undefined): DeepLinkPayload {
  if (!url) return { kind: "unknown", url: "" };
  try {
    const parsed = Linking.parse(url);
    const path = parsed.path ?? "";
    if (path === "auth/callback" || path === "/auth/callback") {
      const token =
        (parsed.queryParams && typeof parsed.queryParams.token === "string"
          ? parsed.queryParams.token
          : null) ?? "";
      if (token) return { kind: "auth-callback", token };
    }
    return { kind: "unknown", url };
  } catch {
    return { kind: "unknown", url };
  }
}

// Returns a subscription. Components that need imperative handling (e.g. the
// fallback auth screen) can call this instead of relying on the router.
export function subscribeToDeepLinks(
  cb: (payload: DeepLinkPayload) => void,
): () => void {
  const sub = Linking.addEventListener("url", ({ url }) => cb(parseDeepLink(url)));
  void Linking.getInitialURL().then((url) => {
    if (url) cb(parseDeepLink(url));
  });
  return () => sub.remove();
}

export function redirectUri(): string {
  return Linking.createURL("/auth/callback");
}
