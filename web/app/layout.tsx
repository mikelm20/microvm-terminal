import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Learn Claude Code",
  description: "Interactive, browser-based environment to learn Claude Code hands-on.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className="min-h-screen bg-learn-bg text-learn-cream antialiased">
        {children}
      </body>
    </html>
  );
}
