import * as React from "react";
import type { Metadata, Viewport } from "next";
import "./globals.css";

export const metadata: Metadata = {
  metadataBase: new URL(
    process.env.NEXT_PUBLIC_SITE_URL || "https://learn.example.com",
  ),
  title: {
    default: "learn.example.com",
    template: "%s . learn.example.com",
  },
  description: "Aprende Claude Code usandolo. Tres minutos al dia.",
  applicationName: "learn.example.com",
  authors: [{ name: "Platform Engineering" }],
  manifest: "/manifest",
  openGraph: {
    type: "website",
    siteName: "learn.example.com",
    url: "/",
  },
  twitter: {
    card: "summary_large_image",
  },
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  viewportFit: "cover",
  themeColor: "#6e2608",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}): React.ReactElement {
  const posthogKey = process.env.NEXT_PUBLIC_POSTHOG_KEY;
  const posthogHost = process.env.NEXT_PUBLIC_POSTHOG_HOST || "https://eu.i.posthog.com";
  return (
    <html lang="es">
      <body>
        {children}
        {posthogKey ? (
          <script
            // PostHog JS snippet. Only loaded when a key is configured, so dev
            // builds and CI remain fully offline.
            dangerouslySetInnerHTML={{
              __html: `
!function(){var t=window.posthog=window.posthog||[];t._i=[];t.init=function(e,o){var n=document.createElement('script');n.type='text/javascript';n.async=!0;n.src='${posthogHost}/static/array.js';var s=document.getElementsByTagName('script')[0];s.parentNode.insertBefore(n,s);var c=t;for(var p=['capture','identify','alias','people','set','set_once','register','register_once','unregister','opt_out_capturing','has_opted_out_capturing','opt_in_capturing','reset','isFeatureEnabled','onFeatureFlags','getFeatureFlag','getFeatureFlagPayload','reloadFeatureFlags','group','updateEarlyAccessFeatureEnrollment','getEarlyAccessFeatures','getActiveMatchingSurveys','getSurveys','getNextSurveyStep','on','onSessionId'],l=0;l<p.length;l++)g(c,p[l]);t._i.push([e,o,{}])};t.init('${posthogKey}',{api_host:'${posthogHost}',person_profiles:'identified_only',capture_pageview:true});function g(t,e){t[e]=function(){t.push([e].concat(Array.prototype.slice.call(arguments,0)))}}}();
              `,
            }}
          />
        ) : null}
      </body>
    </html>
  );
}
