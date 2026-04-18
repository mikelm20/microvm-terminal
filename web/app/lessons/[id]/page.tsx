import { notFound } from "next/navigation";
import AuthGate from "@/components/AuthGate";
import LessonPlayer from "@/components/LessonPlayer";
import { getLesson, lessonExists, type Lang } from "@/lib/lessons";

type Params = { id: string };
type SearchParams = { lang?: string };

export default async function LessonPage({
  params,
  searchParams,
}: {
  params: Promise<Params>;
  searchParams: Promise<SearchParams>;
}) {
  const { id } = await params;
  const { lang: rawLang } = await searchParams;
  const lang: Lang = rawLang === "en" ? "en" : "es";

  if (!lessonExists(id)) {
    notFound();
  }

  // Fall back to the other language if the requested one isn't authored yet.
  const lesson = getLesson(id, lang) ?? getLesson(id, lang === "es" ? "en" : "es");
  if (!lesson) notFound();

  return (
    <AuthGate>
      <LessonPlayer lesson={lesson} />
    </AuthGate>
  );
}
