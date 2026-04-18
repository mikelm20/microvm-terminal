import AuthGate from "@/components/AuthGate";
import LessonsCatalog from "@/components/LessonsCatalog";
import { listLessons } from "@/lib/lessons";

export const dynamic = "force-static";

export default function LessonsPage() {
  const lessonsEs = listLessons("es");
  const lessonsEn = listLessons("en");
  return (
    <AuthGate>
      <LessonsCatalog lessonsEs={lessonsEs} lessonsEn={lessonsEn} />
    </AuthGate>
  );
}
