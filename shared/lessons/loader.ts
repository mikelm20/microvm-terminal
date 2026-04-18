import yaml from "js-yaml";
import { LessonSchema, type Lesson } from "./schema";

export function parseLessonYaml(source: string): Lesson {
  const raw = yaml.load(source);
  return LessonSchema.parse(raw);
}
