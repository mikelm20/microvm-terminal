import "server-only";
import fs from "node:fs";
import path from "node:path";
import { parseLessonYaml, type Lesson } from "@learn/shared-lessons";

// Discovery strategy:
//
// 1. Prefer an exported registry from `@learn/shared-lessons` if one ever
//    appears (shape: `{ pillars: PillarDef[] }`). Accessed via dynamic import
//    so we do not hard-fail typecheck if the module does not exist yet.
// 2. Fallback: read `lessons/` from disk. Any YAML whose `id` starts with
//    `p1-`, `p2-`, or `p3-` is slotted into the matching pillar. Spanish
//    (`.es.yml`) is preferred over English (`.en.yml`); untagged `.yml`
//    (like `hello-claude.yml`) is accepted as a neutral fallback.
//
// The three pillars are intentionally hardcoded in their display order,
// but the *lesson ids* inside them are discovered, never hardcoded.

export type PillarId = "p1" | "p2" | "p3";

export type PillarDef = {
  id: PillarId;
  title: string;
  subtitle: string;
};

export type PillarWithLessons = PillarDef & {
  lessons: Lesson[];
};

export const PILLARS: readonly PillarDef[] = [
  {
    id: "p1",
    title: "Comandos",
    subtitle: "Los primeros gestos en la terminal.",
  },
  {
    id: "p2",
    title: "Context management",
    subtitle: "Darle a Claude lo que necesita saber.",
  },
  {
    id: "p3",
    title: "Prompting de calidad",
    subtitle: "Pedir con contexto, objetivo y tono.",
  },
] as const;

const LESSON_DIR_CANDIDATES = [
  // From apps/web working dir at runtime in Next.
  path.resolve(process.cwd(), "lessons"),
  path.resolve(process.cwd(), "..", "..", "lessons"),
  path.resolve(process.cwd(), "..", "lessons"),
];

function resolveLessonsDir(): string | null {
  for (const candidate of LESSON_DIR_CANDIDATES) {
    if (fs.existsSync(candidate) && fs.statSync(candidate).isDirectory()) {
      return candidate;
    }
  }
  return null;
}

function listYamlFiles(dir: string): string[] {
  return fs
    .readdirSync(dir)
    .filter((f) => f.endsWith(".yml") || f.endsWith(".yaml"))
    .sort();
}

type LessonCandidate = {
  file: string;
  priority: number; // lower is better
};

function candidatePriority(file: string): number {
  if (file.endsWith(".es.yml") || file.endsWith(".es.yaml")) return 0;
  if (file.endsWith(".en.yml") || file.endsWith(".en.yaml")) return 2;
  return 1; // plain .yml, like hello-claude.yml
}

function tryParse(file: string): Lesson | null {
  try {
    const raw = fs.readFileSync(file, "utf8");
    return parseLessonYaml(raw);
  } catch {
    return null;
  }
}

function pillarIdForLesson(lessonId: string): PillarId | null {
  if (lessonId.startsWith("p1-")) return "p1";
  if (lessonId.startsWith("p2-")) return "p2";
  if (lessonId.startsWith("p3-")) return "p3";
  return null;
}

function loadAllLessons(): Lesson[] {
  const dir = resolveLessonsDir();
  if (!dir) return [];
  const files = listYamlFiles(dir);

  // Keep, per `lesson.id`, the best-priority parse.
  const best = new Map<string, LessonCandidate & { lesson: Lesson }>();
  for (const f of files) {
    const full = path.join(dir, f);
    const lesson = tryParse(full);
    if (!lesson) continue;
    const priority = candidatePriority(f);
    const current = best.get(lesson.id);
    if (!current || priority < current.priority) {
      best.set(lesson.id, { file: full, priority, lesson });
    }
  }
  return Array.from(best.values()).map((c) => c.lesson);
}

type ExternalRegistry = {
  pillars?: ReadonlyArray<{
    id: PillarId;
    lessonIds: readonly string[];
  }>;
};

async function loadExternalRegistry(): Promise<ExternalRegistry | null> {
  try {
    // Dynamic, so missing file does not break the build.
    const mod = (await import(
      /* webpackIgnore: true */ "@learn/shared-lessons/registry" as string
    )) as ExternalRegistry | { default?: ExternalRegistry };
    if ("pillars" in mod && Array.isArray(mod.pillars)) {
      return mod as ExternalRegistry;
    }
    if ("default" in mod && mod.default && Array.isArray(mod.default.pillars)) {
      return mod.default;
    }
    return null;
  } catch {
    return null;
  }
}

export async function loadPillars(): Promise<PillarWithLessons[]> {
  const all = loadAllLessons();
  const byId = new Map(all.map((l) => [l.id, l]));

  const external = await loadExternalRegistry();
  if (external && external.pillars && external.pillars.length > 0) {
    return PILLARS.map((p) => {
      const ext = external.pillars?.find((e) => e.id === p.id);
      const lessons = ext
        ? ext.lessonIds
            .map((id) => byId.get(id))
            .filter((l): l is Lesson => Boolean(l))
        : [];
      return { ...p, lessons };
    });
  }

  // Filesystem fallback, grouped by id prefix.
  const buckets = new Map<PillarId, Lesson[]>();
  for (const p of PILLARS) buckets.set(p.id, []);
  for (const lesson of all) {
    const pid = pillarIdForLesson(lesson.id);
    if (!pid) continue;
    buckets.get(pid)?.push(lesson);
  }
  // Sort inside each bucket by lesson id (p1-01, p1-02, ...).
  for (const [pid, list] of buckets) {
    list.sort((a, b) => a.id.localeCompare(b.id));
    buckets.set(pid, list);
  }

  return PILLARS.map((p) => ({
    ...p,
    lessons: buckets.get(p.id) ?? [],
  }));
}

export async function loadLessonById(lessonId: string): Promise<Lesson | null> {
  const all = loadAllLessons();
  return all.find((l) => l.id === lessonId) ?? null;
}
