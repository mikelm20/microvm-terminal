import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

const SUPPORTED = ["es", "en"] as const;
const DEFAULT_LANG = "es";
const BYPASS_PREFIXES = [
  "/og",
  "/verify",
  "/api",
  "/_next",
  "/robots.txt",
  "/sitemap.xml",
  "/favicon.ico",
  "/icon",
  "/apple-icon",
  "/opengraph-image",
  "/manifest",
  "/manifest.webmanifest",
  "/og-fallback.png",
];

function negotiate(accept: string | null | undefined): string {
  if (!accept) return DEFAULT_LANG;
  const ranked = accept
    .split(",")
    .map((chunk) => {
      const [tag, qPart] = chunk.split(";");
      const q = qPart?.trim().startsWith("q=")
        ? parseFloat(qPart.trim().slice(2))
        : 1;
      return { tag: (tag ?? "").trim().toLowerCase(), q: Number.isFinite(q) ? q : 1 };
    })
    .filter((t) => t.tag.length > 0)
    .sort((a, b) => b.q - a.q);
  for (const { tag } of ranked) {
    const primary = tag.split("-")[0];
    if (primary && (SUPPORTED as readonly string[]).includes(primary)) {
      return primary;
    }
  }
  return DEFAULT_LANG;
}

export function middleware(req: NextRequest): NextResponse {
  const { pathname } = req.nextUrl;
  if (BYPASS_PREFIXES.some((p) => pathname === p || pathname.startsWith(p + "/"))) {
    return NextResponse.next();
  }
  if (pathname.includes(".") && !pathname.startsWith("/_next/")) {
    // static asset with extension, let it through
    return NextResponse.next();
  }
  const first = pathname.split("/")[1] ?? "";
  if ((SUPPORTED as readonly string[]).includes(first)) {
    return NextResponse.next();
  }
  const lang = negotiate(req.headers.get("accept-language"));
  const next = req.nextUrl.clone();
  next.pathname = `/${lang}${pathname === "/" ? "" : pathname}`;
  return NextResponse.redirect(next);
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
