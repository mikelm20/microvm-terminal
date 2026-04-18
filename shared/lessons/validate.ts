#!/usr/bin/env -S npx tsx
// Validate every YAML under /lessons against the LessonSchema declared here.
// Exits 0 on pass, 1 on any failure.
//
//   pnpm --filter @learn/shared-lessons run validate
// or from repo root:
//   npx tsx shared/lessons/validate.ts
import fs from "node:fs";
import path from "node:path";
import url from "node:url";
import yaml from "js-yaml";
import { LessonSchema } from "./schema";

const __dirname = path.dirname(url.fileURLToPath(import.meta.url));
const lessonsDir = path.resolve(__dirname, "..", "..", "lessons");

const files = fs
  .readdirSync(lessonsDir)
  .filter((f) => f.endsWith(".yml") || f.endsWith(".yaml"))
  .sort();

let failed = 0;
for (const f of files) {
  const full = path.join(lessonsDir, f);
  const src = fs.readFileSync(full, "utf8");
  try {
    const raw = yaml.load(src);
    const parsed = LessonSchema.safeParse(raw);
    if (!parsed.success) {
      failed++;
      console.error(`FAIL ${f}`);
      for (const iss of parsed.error.issues) {
        console.error(`  ${iss.path.join(".")}: ${iss.message}`);
      }
    } else {
      console.log(`ok   ${f}`);
    }
  } catch (e) {
    failed++;
    console.error(`FAIL ${f} (yaml): ${(e as Error).message}`);
  }
}

if (failed > 0) {
  console.error(`\n${failed} file(s) failed`);
  process.exit(1);
}
console.log(`\nAll ${files.length} lesson(s) valid.`);
