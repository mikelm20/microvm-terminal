import type {
  PublicProfileResponse,
  PublicCertificateResponse,
} from "@learn/shared-api/schemas";

/**
 * Seeded fixtures used when the control plane is unreachable or still wiring up
 * its `/p/:uuid/public` and `/certificate/:id/public` endpoints. TODO: remove
 * the local fallback once Agent-API lands the public surface.
 */
export const SEED_PROFILE_UUID = "00000000-0000-4000-8000-000000000001";
export const SEED_CERTIFICATE_ID = "demo-cert-2026-04-18";

export const SEED_PROFILE: PublicProfileResponse = {
  uuid: SEED_PROFILE_UUID,
  name: "Marta Ruiz",
  department: "ventas",
  lang: "es",
  streak_days: 12,
  total_xp: 680,
  modules_completed: [
    {
      lesson_id: "m2-primera-conversacion",
      title: "Tu primera conversacion con Claude",
      completed_at: "2026-04-06T16:02:00Z",
      takeaways: [
        "Claude lee archivos antes de responder.",
        "Los tool calls son el protagonista, no el texto.",
      ],
    },
    {
      lesson_id: "m3-organizar-cabeza",
      title: "Organizar la cabeza con Claude",
      completed_at: "2026-04-11T15:45:00Z",
      takeaways: [
        "Un prompt claro viene de una mente clara.",
        "Pide la tabla de contenidos antes del capitulo.",
      ],
    },
    {
      lesson_id: "m4-investigar-rapido",
      title: "Investigar rapido sin perderse",
      completed_at: "2026-04-17T19:10:00Z",
      takeaways: [
        "WebFetch + Grep es un superpoder discreto.",
        "Cerrar la busqueda con un resumen evita volver al punto cero.",
      ],
    },
  ],
};

export const SEED_CERTIFICATE: PublicCertificateResponse = {
  id: SEED_CERTIFICATE_ID,
  uuid: SEED_PROFILE_UUID,
  name: "Marta Ruiz",
  department: "ventas",
  lang: "es",
  completed_at: "2026-04-17T19:10:00Z",
  // Dev-only Ed25519 signature over certificatePayload(SEED_CERTIFICATE). The
  // matching public key lives in apps/web/.env.example as
  // NEXT_PUBLIC_CERT_VERIFY_PUBKEY for local demos. Agent-API replaces both
  // with real, per-environment keys managed via Doppler.
  signature:
    "c2043734aa4e1bef7edee2226ac473593fe3bf213923dd8087abe222325cf03e4562f499b7d0c127fd756e142566dc092f2b911c6e663aa1655139ac6d7cec0e",
  modules: [
    "m2-primera-conversacion",
    "m3-organizar-cabeza",
    "m4-investigar-rapido",
  ],
};

// The public half of the dev keypair that signed SEED_CERTIFICATE.signature.
// Only used when NEXT_PUBLIC_CERT_VERIFY_PUBKEY is not set in the environment.
export const SEED_CERT_PUBKEY_HEX =
  "feb876f133f5400d6f8ec3c021fddd4cb8210712970baaffa1abde8ef8eedc9e";
