import type { PublicCertificateResponse } from "@learn/shared-api/schemas";

/**
 * Canonicalizes the fields the control plane signs. Same order the Go signer
 * MUST use (Agent-API): `id|uuid|lesson_id_csv|completed_at`.
 */
export function certificatePayload(cert: PublicCertificateResponse): string {
  const modules = [...cert.modules].sort().join(",");
  return [cert.id, cert.uuid, modules, cert.completed_at].join("|");
}

export function hexToBytes(hex: string): Uint8Array {
  const clean = hex.replace(/^0x/i, "").trim();
  if (clean.length % 2 !== 0) throw new Error("odd hex length");
  const out = new Uint8Array(clean.length / 2);
  for (let i = 0; i < out.length; i++) {
    const pair = clean.slice(i * 2, i * 2 + 2);
    const byte = parseInt(pair, 16);
    if (Number.isNaN(byte)) throw new Error("bad hex");
    out[i] = byte;
  }
  return out;
}

export function bytesToHex(bytes: Uint8Array): string {
  let out = "";
  for (let i = 0; i < bytes.length; i++) {
    const b = bytes[i];
    if (b === undefined) continue;
    out += b.toString(16).padStart(2, "0");
  }
  return out;
}

export function encodeUtf8(str: string): Uint8Array {
  return new TextEncoder().encode(str);
}
