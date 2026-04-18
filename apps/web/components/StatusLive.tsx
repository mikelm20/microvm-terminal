"use client";

import * as React from "react";
import type { Lang } from "@/lib/i18n";

type Health = { ok: boolean | null; warmPoolDepth: number | null; checkedAt: Date | null };

async function fetchHealth(): Promise<Health> {
  const base =
    process.env.NEXT_PUBLIC_CONTROL_PLANE_URL || "https://api.learn.example.com";
  try {
    const res = await fetch(`${base.replace(/\/$/, "")}/healthz`, { cache: "no-store" });
    if (!res.ok) return { ok: false, warmPoolDepth: null, checkedAt: new Date() };
    const data = (await res.json()) as { ok?: boolean; warm_pool_depth?: number };
    return {
      ok: !!data?.ok,
      // Warm pool depth is stubbed until Agent-API extends the /healthz shape.
      warmPoolDepth: typeof data?.warm_pool_depth === "number" ? data.warm_pool_depth : null,
      checkedAt: new Date(),
    };
  } catch {
    return { ok: false, warmPoolDepth: null, checkedAt: new Date() };
  }
}

export function StatusLive({ lang }: { lang: Lang }): React.ReactElement {
  const [health, setHealth] = React.useState<Health>({
    ok: null,
    warmPoolDepth: null,
    checkedAt: null,
  });

  React.useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const tick = async () => {
      const h = await fetchHealth();
      if (!cancelled) {
        setHealth(h);
        timer = setTimeout(tick, 15000);
      }
    };
    void tick();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, []);

  const labels =
    lang === "es"
      ? {
          controlPlane: "Control plane",
          warmPool: "VMs calientes",
          lastChecked: "Ultima comprobacion",
          up: "Arriba",
          down: "Caido",
          loading: "Comprobando",
          unavailable: "sin dato",
        }
      : {
          controlPlane: "Control plane",
          warmPool: "Warm VMs",
          lastChecked: "Last checked",
          up: "Up",
          down: "Down",
          loading: "Checking",
          unavailable: "n/a",
        };

  const okBadge = (() => {
    if (health.ok === null) {
      return (
        <span className="learn-pill" style={{ color: "#e6cba3" }}>
          {labels.loading}
        </span>
      );
    }
    if (health.ok) {
      return (
        <span
          className="learn-pill"
          style={{ background: "rgba(111, 207, 122, 0.16)", color: "#6fcf7a" }}
        >
          {labels.up}
        </span>
      );
    }
    return (
      <span
        className="learn-pill"
        style={{ background: "rgba(230, 114, 74, 0.16)", color: "#ff9b5a" }}
      >
        {labels.down}
      </span>
    );
  })();

  const checkedAt = health.checkedAt
    ? health.checkedAt.toISOString().replace("T", " ").slice(0, 19) + " UTC"
    : labels.loading;

  return (
    <section className="learn-card space-y-4">
      <div className="flex items-center justify-between">
        <div className="text-learn-warmHi">{labels.controlPlane}</div>
        {okBadge}
      </div>
      <div className="flex items-center justify-between">
        <div className="text-learn-warmHi">{labels.warmPool}</div>
        <div className="font-mono text-learn-ember">
          {health.warmPoolDepth !== null ? String(health.warmPoolDepth) : labels.unavailable}
        </div>
      </div>
      <div className="text-sm text-learn-warm/80">
        {labels.lastChecked}: <span className="font-mono">{checkedAt}</span>
      </div>
    </section>
  );
}
