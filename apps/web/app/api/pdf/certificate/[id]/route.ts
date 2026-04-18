import { PDFDocument, StandardFonts, rgb } from "pdf-lib";
import { getPublicCertificate } from "@/lib/api";
import { SEED_CERTIFICATE, SEED_CERTIFICATE_ID } from "@/lib/mock";

export const runtime = "nodejs";
export const revalidate = 60;

export async function GET(
  _req: Request,
  ctx: { params: Promise<{ id: string }> },
): Promise<Response> {
  const { id } = await ctx.params;
  let cert = null as Awaited<ReturnType<typeof getPublicCertificate>>;
  try {
    cert = await getPublicCertificate(id);
  } catch {
    cert = null;
  }
  if (!cert && id === SEED_CERTIFICATE_ID) cert = SEED_CERTIFICATE;
  if (!cert) return new Response("not found", { status: 404 });

  const pdf = await PDFDocument.create();
  const page = pdf.addPage([842, 595]); // A4 landscape in points
  const cream = rgb(1, 0.964, 0.902); // #fff6e6
  const burgundy = rgb(0.431, 0.149, 0.031); // #6e2608
  const flame = rgb(1, 0.608, 0.353); // #ff9b5a

  page.drawRectangle({
    x: 0,
    y: 0,
    width: 842,
    height: 595,
    color: cream,
  });
  page.drawRectangle({
    x: 40,
    y: 40,
    width: 762,
    height: 515,
    borderColor: burgundy,
    borderWidth: 1,
    color: cream,
  });
  page.drawCircle({ x: 80, y: 520, size: 12, color: flame });

  const font = await pdf.embedFont(StandardFonts.Helvetica);
  const fontBold = await pdf.embedFont(StandardFonts.HelveticaBold);
  const fontMono = await pdf.embedFont(StandardFonts.Courier);

  page.drawText("LEARN.EXAMPLE.COM", {
    x: 110,
    y: 515,
    size: 12,
    font,
    color: burgundy,
  });

  page.drawText("Certificate of completion", {
    x: 80,
    y: 450,
    size: 18,
    font,
    color: burgundy,
  });

  const name = cert.name ?? "Learner";
  page.drawText(name, {
    x: 80,
    y: 390,
    size: 44,
    font: fontBold,
    color: rgb(0.165, 0.07, 0.031), // deep
  });

  const course = cert.lang === "es" ? "Fundamentos de Claude Code" : "Claude Code Fundamentals";
  page.drawText(course, {
    x: 80,
    y: 340,
    size: 18,
    font,
    color: burgundy,
  });

  const date = cert.completed_at.slice(0, 10);
  page.drawText(`Completed on ${date}`, {
    x: 80,
    y: 310,
    size: 14,
    font,
    color: burgundy,
  });

  let y = 260;
  page.drawText("Modules", { x: 80, y, size: 11, font: fontBold, color: burgundy });
  y -= 18;
  for (const m of cert.modules) {
    page.drawText("- " + m, { x: 80, y, size: 11, font, color: burgundy });
    y -= 14;
  }

  page.drawText("Signed by Mikel Martin . Platform Engineering", {
    x: 80,
    y: 120,
    size: 11,
    font,
    color: burgundy,
  });
  page.drawText("ID: " + cert.id, {
    x: 80,
    y: 100,
    size: 10,
    font: fontMono,
    color: burgundy,
  });
  page.drawText("UUID: " + cert.uuid, {
    x: 80,
    y: 85,
    size: 10,
    font: fontMono,
    color: burgundy,
  });
  page.drawText("Signature (Ed25519):", {
    x: 80,
    y: 68,
    size: 10,
    font: fontBold,
    color: burgundy,
  });
  page.drawText(cert.signature.slice(0, 96), {
    x: 80,
    y: 54,
    size: 8,
    font: fontMono,
    color: burgundy,
  });
  if (cert.signature.length > 96) {
    page.drawText(cert.signature.slice(96), {
      x: 80,
      y: 44,
      size: 8,
      font: fontMono,
      color: burgundy,
    });
  }

  const verifyUrl = `${process.env.NEXT_PUBLIC_SITE_URL || "https://learn.example.com"}/verify/${cert.id}`;
  page.drawText("Verify at: " + verifyUrl, {
    x: 80,
    y: 28,
    size: 10,
    font,
    color: burgundy,
  });

  const bytes = await pdf.save();
  const body = new Uint8Array(bytes);
  return new Response(body, {
    status: 200,
    headers: {
      "Content-Type": "application/pdf",
      "Cache-Control": "public, max-age=60, s-maxage=60, stale-while-revalidate=600",
      "Content-Disposition": `inline; filename="certificate-${cert.id}.pdf"`,
    },
  });
}
