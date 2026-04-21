// Fase 2 "Terminal guiada" pillar-to-lesson registry.
//
// Three pillars in pedagogical order:
//   1. comandos    Mechanic: claude --print, interactive, discovery slashes
//   2. context     Context window, /clear + /compact, CLAUDE.md + /resume, subagents
//   3. prompting   Context-goal-tone, examples + critique, decomposition
//
// The lesson ids in each Pillar.lessons array must match the `id` field of a
// YAML file in /lessons. F2_LESSON_ORDER is the canonical flat linearization.
// Client surfaces (path, progress, next-up) consume these two constants.

export type F2PillarId = "comandos" | "context" | "prompting";

export type Pillar = {
  id: F2PillarId;
  title_key: string;
  subtitle_key: string;
  label_key: string;
  intro_key: string;
  complete_key: string;
  lessons: string[];
};

export const F2_PILLARS: readonly Pillar[] = [
  {
    id: "comandos",
    title_key: "taller.pillars.comandos.title",
    subtitle_key: "taller.pillars.comandos.subtitle",
    label_key: "taller.pillars.comandos.label",
    intro_key: "taller.pillars.comandos.intro",
    complete_key: "taller.pillar_complete.comandos",
    lessons: [
      "p1-comandos-01-primer-prompt",
      "p1-comandos-02-interactivo",
      "p1-comandos-03-discovery",
    ],
  },
  {
    id: "context",
    title_key: "taller.pillars.context.title",
    subtitle_key: "taller.pillars.context.subtitle",
    label_key: "taller.pillars.context.label",
    intro_key: "taller.pillars.context.intro",
    complete_key: "taller.pillar_complete.context",
    lessons: [
      "p2-contexto-01-ventana",
      "p2-contexto-02-clear-compact",
      "p2-contexto-03-claude-md",
      "p2-contexto-04-subagentes",
    ],
  },
  {
    id: "prompting",
    title_key: "taller.pillars.prompting.title",
    subtitle_key: "taller.pillars.prompting.subtitle",
    label_key: "taller.pillars.prompting.label",
    intro_key: "taller.pillars.prompting.intro",
    complete_key: "taller.pillar_complete.prompting",
    lessons: [
      "p3-prompting-01-contexto-objetivo-tono",
      "p3-prompting-02-ejemplos-y-critica",
      "p3-prompting-03-descomponer",
    ],
  },
] as const;

export const F2_LESSON_ORDER: readonly string[] = F2_PILLARS.flatMap(
  (p) => p.lessons,
);

export function pillarForLesson(lessonId: string): F2PillarId | undefined {
  for (const p of F2_PILLARS) {
    if (p.lessons.includes(lessonId)) return p.id;
  }
  return undefined;
}

export function nextLessonAfter(lessonId: string): string | undefined {
  const i = F2_LESSON_ORDER.indexOf(lessonId);
  if (i < 0 || i >= F2_LESSON_ORDER.length - 1) return undefined;
  return F2_LESSON_ORDER[i + 1];
}
