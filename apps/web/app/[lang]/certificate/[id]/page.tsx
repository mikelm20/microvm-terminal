import * as React from "react";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import Link from "next/link";
import { coerceLang, fmt, isLang, voiceFor } from "@/lib/i18n";
import { getPublicCertificate } from "@/lib/api";
import { SEED_CERTIFICATE, SEED_CERTIFICATE_ID } from "@/lib/mock";
import { linkedinAddCertificateUrl } from "@/lib/deep-link";

type PageParams = { lang: string; id: string };

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
  const { lang: rawLang, id } = await params;
  if (!isLang(rawLang)) return {};
  const lang = coerceLang(rawLang);
  const cert = await load(id);
  if (!cert) return { title: lang === "es" ? "Certificado" : "Certificate" };
  const voice = voiceFor(lang);
  return {
    title: `${cert.name ?? "learner"} . ${
      lang === "es" ? "certificado" : "certificate"
    }`,
    description: voice.share.body_certificate,
    openGraph: {
      title: `${cert.name ?? "learner"} . learn.example.com`,
      description: voice.share.body_certificate,
      images: [
        { url: `/og/certificate/${id}`, width: 1200, height: 630 },
      ],
    },
    twitter: {
      card: "summary_large_image",
      title: `${cert.name ?? "learner"} . learn.example.com`,
      description: voice.share.body_certificate,
      images: [`/og/certificate/${id}`],
    },
  };
}

export default async function CertificatePage({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<React.ReactElement> {
  const { lang: rawLang, id } = await params;
  if (!isLang(rawLang)) notFound();
  const lang = coerceLang(rawLang);
  const voice = voiceFor(lang);
  const cert = await load(id);
  if (!cert) notFound();

  const completedDate = new Date(cert.completed_at);
  const formattedDate =
    lang === "es"
      ? completedDate.toLocaleDateString("es-ES", {
          day: "numeric",
          month: "long",
          year: "numeric",
        })
      : completedDate.toLocaleDateString("en-US", {
          day: "numeric",
          month: "long",
          year: "numeric",
        });

  const base = process.env.NEXT_PUBLIC_SITE_URL || "https://learn.example.com";
  const certUrl = `${base}/verify/${cert.id}`;
  const linkedinUrl = linkedinAddCertificateUrl({
    name: lang === "es" ? "Fundamentos de Claude Code" : "Claude Code Fundamentals",
    issueDate: completedDate,
    certificateId: cert.id,
    certificateUrl: certUrl,
  });

  return (
    <article className="mx-auto max-w-4xl px-6 py-16 space-y-10">
      <section
        className="mx-auto w-full max-w-3xl rounded-learn-card p-10 sm:p-14 shadow-learn-lift"
        style={{
          background: "#fff6e6",
          color: "#2a1208",
          borderTop: "1px solid #6e2608",
        }}
      >
        <header className="flex items-start justify-between">
          <div>
            <div
              aria-hidden
              className="h-8 w-8 rounded-full"
              style={{
                backgroundImage:
                  "linear-gradient(135deg, #ffc591 0%, #ff9b5a 50%, #e6724a 100%)",
                boxShadow: "0 0 0 4px rgba(255, 197, 145, 0.25)",
              }}
            />
            <p className="mt-4 text-xs tracking-[0.18em] uppercase text-[#6e2608]">
              learn.example.com
            </p>
          </div>
          <div className="text-right text-sm font-mono text-[#6e2608]">
            ID {cert.id}
          </div>
        </header>

        <div
          aria-hidden
          className="my-6 h-px w-full"
          style={{ background: "#6e2608" }}
        />

        <div className="space-y-6">
          <div>
            <p className="text-xs tracking-[0.18em] uppercase text-[#6e2608]">
              {voice.certificate.issued_to}
            </p>
            <p className="mt-1 text-4xl leading-tight text-[#2a1208]">
              {cert.name ?? (lang === "es" ? "Aprendiz" : "Learner")}
            </p>
          </div>

          <div>
            <p className="text-xs tracking-[0.18em] uppercase text-[#6e2608]">
              {voice.certificate.completed}
            </p>
            <p className="mt-1 text-lg">
              {lang === "es"
                ? "Fundamentos de Claude Code"
                : "Claude Code Fundamentals"}{" "}
              {fmt(voice.certificate.on_date, { date: formattedDate })}
            </p>
          </div>

          {cert.modules.length > 0 ? (
            <div>
              <p className="text-xs tracking-[0.18em] uppercase text-[#6e2608]">
                {lang === "es" ? "Modulos" : "Modules"}
              </p>
              <ul className="mt-2 list-disc pl-5 text-sm text-[#2a1208]">
                {cert.modules.map((m) => (
                  <li key={m}>{m}</li>
                ))}
              </ul>
            </div>
          ) : null}

          <div className="flex flex-wrap items-end justify-between gap-4 pt-4">
            <div>
              <p className="text-xs tracking-[0.18em] uppercase text-[#6e2608]">
                {lang === "es" ? "Firmado por" : "Signed by"}
              </p>
              <p className="mt-1 text-base">Mikel Martin . Platform Engineering</p>
            </div>
            <div className="text-right">
              <p className="text-xs tracking-[0.18em] uppercase text-[#6e2608]">UUID</p>
              <p className="font-mono text-xs break-all">{cert.uuid}</p>
            </div>
          </div>

          <div>
            <p className="text-xs tracking-[0.18em] uppercase text-[#6e2608]">
              {lang === "es" ? "Firma Ed25519" : "Ed25519 signature"}
            </p>
            <p className="font-mono text-xs break-all mt-1">{cert.signature}</p>
          </div>
        </div>
      </section>

      <section className="flex flex-wrap gap-3 justify-center">
        <Link
          href={certUrl}
          className="learn-button-primary"
          rel="noreferrer"
        >
          {voice.certificate.verify_cta}
        </Link>
        <Link
          href={linkedinUrl}
          className="learn-button-secondary"
          target="_blank"
          rel="noreferrer"
        >
          {voice.certificate.add_to_linkedin}
        </Link>
        <Link
          href={`/api/pdf/certificate/${cert.id}`}
          className="learn-button-secondary"
          rel="noreferrer"
        >
          {voice.certificate.download_pdf}
        </Link>
      </section>
    </article>
  );
}
