"use client";

import type { PostHog } from "posthog-js";

let clientPromise: Promise<PostHog | null> | null = null;

function posthogKey(): string | undefined {
  return process.env.NEXT_PUBLIC_POSTHOG_KEY?.trim() || undefined;
}

function posthogHost(): string {
  return (
    process.env.NEXT_PUBLIC_POSTHOG_HOST?.trim() || "https://eu.i.posthog.com"
  );
}

async function getClient(): Promise<PostHog | null> {
  if (typeof window === "undefined") return null;
  const key = posthogKey();
  if (!key) return null;
  if (clientPromise) return clientPromise;
  clientPromise = import("posthog-js").then((mod) => {
    const ph = mod.default;
    ph.init(key, {
      api_host: posthogHost(),
      capture_pageview: true,
      persistence: "localStorage",
    });
    return ph;
  });
  return clientPromise;
}

export async function track(
  event: string,
  properties?: Record<string, unknown>,
): Promise<void> {
  const client = await getClient();
  if (!client) return;
  client.capture(event, properties);
}

export async function identify(
  distinctId: string,
  properties?: Record<string, unknown>,
): Promise<void> {
  const client = await getClient();
  if (!client) return;
  client.identify(distinctId, properties);
}
