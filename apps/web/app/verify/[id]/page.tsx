import * as React from "react";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { getPublicCertificate } from "@/lib/api";
import { SEED_CERTIFICATE, SEED_CERTIFICATE_ID, SEED_CERT_PUBKEY_HEX } from "@/lib/mock";
import { VerifyClient } from "@/components/VerifyClient";

type PageParams = { id: string };

async function load(id: string) {
  try {
    const cert = await getPublicCertificate(id);
    if (cert) return cert;
  } catch {
    // fall through
  }
  return id === SEED_CERTIFICATE_ID ? SEED_CERTIFICATE : null;
}

export async function generateMetadata({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<Metadata> {
  const { id } = await params;
  return {
    title: `Verify ${id}`,
    description: "Ed25519 certificate verification by Platform Engineering",
  };
}

export default async function VerifyPage({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<React.ReactElement> {
  const { id } = await params;
  const cert = await load(id);
  if (!cert) notFound();

  // Prefer the env public key. Fall back to the dev seed key so the verify
  // demo works out of the box before Agent-API lands production signing.
  const pubkey =
    process.env.NEXT_PUBLIC_CERT_VERIFY_PUBKEY && process.env.NEXT_PUBLIC_CERT_VERIFY_PUBKEY.trim() !== ""
      ? process.env.NEXT_PUBLIC_CERT_VERIFY_PUBKEY
      : SEED_CERT_PUBKEY_HEX;

  return (
    <main className="min-h-screen">
      <div className="mx-auto max-w-3xl px-6 py-16 space-y-10">
        <header className="space-y-2">
          <p className="learn-eyebrow">Certificate verification</p>
          <h1 className="text-4xl leading-tight text-learn-warmHi">
            {cert.name ?? "learner"}
          </h1>
          <p className="text-sm text-learn-warm">
            issued by <span className="text-learn-warmHi">Platform Engineering</span> . learn.example.com
          </p>
        </header>

        <VerifyClient cert={cert} pubkeyHex={pubkey} />

        <section className="learn-card space-y-3 text-sm text-learn-warm">
          <div className="flex justify-between">
            <span>ID</span>
            <span className="font-mono text-learn-warmHi">{cert.id}</span>
          </div>
          <div className="flex justify-between">
            <span>UUID</span>
            <span className="font-mono break-all max-w-[60%] text-right">{cert.uuid}</span>
          </div>
          <div className="flex justify-between">
            <span>Completed at</span>
            <span className="font-mono">{cert.completed_at}</span>
          </div>
          <div className="flex justify-between">
            <span>Modules</span>
            <span className="font-mono text-right">{cert.modules.join(", ")}</span>
          </div>
          <div>
            <div>Signature</div>
            <div className="font-mono text-xs break-all mt-1 text-learn-warmHi">{cert.signature}</div>
          </div>
        </section>
      </div>
    </main>
  );
}
