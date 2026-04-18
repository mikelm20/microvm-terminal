// Lesson types + helpers. Lesson data is generated from ../lessons/*.yml at
// build time by scripts/generate-lessons.mjs.

import { LESSONS } from "./lessons-generated";

export type Lang = "es" | "en";

export type Predicate =
  | { type: "agent_online" }
  | { type: "process_started"; match: { name: string } }
  | { type: "port_listening"; match?: { port?: number } }
  | { type: "claude_prompt_sent"; match_regex?: string; match?: { regex?: string } }
  | { type: "claude_tool_call"; match: { tool: string; path_regex?: string; subagent_type?: string; command_regex?: string } }
  | { type: "claude_slash_command"; match: { command: string } }
  | { type: "file_exists"; match: { path: string } }
  | { type: "file_contents_regex"; match: { path: string; regex: string } };

export type LessonStep = {
  id: string;
  title: string;
  body?: string;
  prompt?: string;
  hint?: string;
  xp: number;
  success?: Predicate;
  success_title?: string;
  success_sub?: string;
};

export type Lesson = {
  id: string;
  module_number: number;
  language: Lang;
  title: string;
  subtitle?: string;
  description?: string;
  estimated_minutes?: number;
  sandbox?: string;
  mental_model?: string;
  preconditions?: {
    working_directory?: string;
    claude_running?: boolean;
    files_present?: string[];
  };
  steps: LessonStep[];
  on_complete?: {
    total_xp?: number;
    takeaways?: string[];
    next_module?: string | null;
    course_complete?: boolean;
    trigger_signup?: boolean;
    milestone_screen?: string;
  };
};

export function listLessons(lang: Lang = "es"): Lesson[] {
  return LESSONS.filter((l) => l.language === lang).sort(
    (a, b) => a.module_number - b.module_number,
  );
}

export function getLesson(id: string, lang: Lang = "es"): Lesson | null {
  return LESSONS.find((l) => l.id === id && l.language === lang) ?? null;
}

export function lessonExists(id: string): boolean {
  return LESSONS.some((l) => l.id === id);
}
