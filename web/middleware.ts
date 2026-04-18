import { NextRequest, NextResponse } from "next/server";

// When deployed to the public VPS (core-01) this binary serves ONLY the
// landing. The gated routes (/sandbox, /lessons, /lessons/[id]) live on the
// bare metal that runs the Firecracker control plane. We forward any direct
// hit to those paths to the bare-metal host so shared URLs keep working.
//
// Guarded by LANDING_ONLY so the same binary running on bare metal does not
// redirect its own routes into a loop.
const SANDBOX_HOST = process.env.SANDBOX_HOST ?? "sandbox.learn.example.com";

export function middleware(req: NextRequest) {
  if (process.env.LANDING_ONLY !== "1") return NextResponse.next();

  const { pathname, search } = req.nextUrl;
  const target = new URL(`https://${SANDBOX_HOST}${pathname}${search}`);
  // 307 keeps method + body. The gated routes are GETs today but this stays
  // safe if we ever add POST /login redirects.
  return NextResponse.redirect(target, 307);
}

// Match everything EXCEPT:
//   /                    (the landing itself)
//   /_next/*             (Next asset bundles)
//   /favicon.ico, images (static assets served directly by Next)
export const config = {
  matcher: [
    "/((?!$|_next/|favicon\\.ico|images/|assets/|.*\\.(?:png|jpg|jpeg|gif|svg|webp|ico|css|js|map|woff2?|ttf)$).*)",
  ],
};
