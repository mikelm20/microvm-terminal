"use client";

import * as React from "react";
import { verifyAsync } from "@noble/ed25519";
import type { PublicCertificateResponse } from "@learn/shared-api/schemas";
import { certificatePayload, encodeUtf8, hexToBytes } from "@/lib/cert";

type Status = "checking" | "verified" | "invalid" | "no_pubkey" | "bad_signature";

export function VerifyClient({
  cert,
  pubkeyHex,
}: {
  cert: PublicCertificateResponse;
  pubkeyHex: string;
}): React.ReactElement {
  const [status, setStatus] = React.useState<Status>("checking");

  React.useEffect(() => {
    let cancelled = false;
    const run = async () => {
      if (!pubkeyHex) {
        if (!cancelled) setStatus("no_pubkey");
        return;
      }
      try {
        const sig = hexToBytes(cert.signature);
        const pub = hexToBytes(pubkeyHex);
        const msg = encodeUtf8(certificatePayload(cert));
        const ok = await verifyAsync(sig, msg, pub);
        if (!cancelled) setStatus(ok ? "verified" : "invalid");
      } catch {
        if (!cancelled) setStatus("bad_signature");
      }
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [cert, pubkeyHex]);

  if (status === "verified") {
    return (
      <section
        className="rounded-learn-card p-6 text-learn-warmHi"
        style={{
          background: "rgba(111, 207, 122, 0.14)",
          border: "1px solid rgba(111, 207, 122, 0.4)",
        }}
      >
        <p className="text-xs tracking-[0.18em] uppercase text-learn-success">
          Verified
        </p>
        <p className="mt-1 text-2xl">This certificate is authentic.</p>
        <p className="mt-2 text-sm text-learn-warm">
          Signature matches the Platform Engineering public key.
        </p>
      </section>
    );
  }

  if (status === "invalid" || status === "bad_signature") {
    return (
      <section
        className="rounded-learn-card p-6 text-learn-warmHi"
        style={{
          background: "rgba(230, 114, 74, 0.14)",
          border: "1px solid rgba(230, 114, 74, 0.5)",
        }}
      >
        <p className="text-xs tracking-[0.18em] uppercase text-learn-attention">
          Invalid
        </p>
        <p className="mt-1 text-2xl">This signature does not verify.</p>
        <p className="mt-2 text-sm text-learn-warm">
          {status === "bad_signature"
            ? "Signature could not be parsed."
            : "Signature does not match the canonical payload for this certificate."}
        </p>
      </section>
    );
  }

  if (status === "no_pubkey") {
    return (
      <section className="learn-card">
        <p className="text-xs tracking-[0.18em] uppercase text-learn-attention">
          Verifier key not configured
        </p>
        <p className="mt-2 text-sm text-learn-warm">
          NEXT_PUBLIC_CERT_VERIFY_PUBKEY is empty. The page cannot validate the
          signature client-side. Configure the public Ed25519 key from Agent-API
          in the environment.
        </p>
      </section>
    );
  }

  return (
    <section className="learn-card">
      <p className="text-xs tracking-[0.18em] uppercase text-learn-warm">
        Checking
      </p>
      <p className="mt-2 text-sm text-learn-warm">Verifying Ed25519 signature.</p>
    </section>
  );
}
