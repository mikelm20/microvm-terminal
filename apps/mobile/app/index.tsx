// Path screen. The home of the app.
//
// Top: StreakRibbon.
// Next: ReturnCard if the learner is returning from a gap.
// Body: vertical column of lesson nodes. Current node at 1.4x with flame
// glow and ambient breathing. Two nodes ahead silhouette at 40% opacity.
// Locked beyond do not render.
//
// Tapping the current node routes into the lesson group owned by
// agent-convo. Lesson data will come from GET /lessons via @learn/shared-api
// once the API is live; for now we ship a placeholder list so the screen
// renders on day one.

import { useMemo } from "react";
import { Text, View } from "react-native";
import { useRouter } from "expo-router";
import { tokens } from "@learn/shared-tokens";
import { useT } from "../lib/i18n";
import {
  useProgress,
  currentReturnBucket,
} from "../lib/progress";
import { StreakRibbon } from "../components/path/StreakRibbon";
import { PathColumn } from "../components/path/PathColumn";
import { PathNode, type NodeKind } from "../components/path/PathNode";
import { ReturnCard } from "../components/return/ReturnCard";

// Placeholder lesson catalog. Replaced by GET /lessons once agent-convo
// ships lib/api.ts. The id must line up with what the lesson route expects.
const CATALOG = [
  { id: "m2-primera-conversacion", label: "Primera conversacion" },
  { id: "m3-organizar-cabeza", label: "Organizar la cabeza" },
  { id: "m4-leer-documentos", label: "Leer documentos" },
  { id: "m5-escribir-correos", label: "Escribir correos" },
  { id: "m6-automatizar", label: "Automatizar tareas" },
  { id: "m7-revisar-todo", label: "Revisar todo" },
] as const;

export default function PathScreen() {
  const t = useT();
  const router = useRouter();
  const progress = useProgress();
  const ret = currentReturnBucket();

  const nodes = useMemo(() => {
    const completedIds = new Set(
      Object.entries(progress.modules)
        .filter(([, m]) => m.completed_steps.length > 0 && m.paused_at === null)
        .map(([id]) => id),
    );
    let currentAssigned = false;
    const rendered: Array<{
      id: string;
      label: string;
      kind: NodeKind;
    }> = [];
    for (let i = 0; i < CATALOG.length; i++) {
      const lesson = CATALOG[i]!;
      let kind: NodeKind;
      if (completedIds.has(lesson.id)) {
        kind = "completed";
      } else if (!currentAssigned) {
        kind = "current";
        currentAssigned = true;
      } else {
        kind = "upcoming";
      }
      // Locked beyond 2 ahead do not render.
      if (
        kind === "upcoming" &&
        rendered.filter((r) => r.kind === "upcoming").length >= 2
      ) {
        continue;
      }
      rendered.push({ id: lesson.id, label: lesson.label, kind });
    }
    return rendered;
  }, [progress.modules]);

  const currentLessonId = nodes.find((n) => n.kind === "current")?.id;

  const openCurrent = () => {
    if (!currentLessonId) return;
    router.push(`/(lesson)/${currentLessonId}/conversation` as never);
  };

  return (
    <PathColumn
      header={
        <View>
          <StreakRibbon
            streak={progress.streak_days}
            graceTokens={progress.grace_tokens}
          />
          <Text
            style={{
              color: tokens.color.ink.primary,
              fontSize: 28,
              fontWeight: "500",
              marginTop: 20,
              letterSpacing: -0.5,
            }}
          >
            {t("path.header")}
          </Text>
          {ret.bucket !== "none" ? (
            <View style={{ marginTop: 20 }}>
              <ReturnCard
                bucket={ret.bucket}
                days={ret.days}
                onPrimary={openCurrent}
              />
            </View>
          ) : null}
        </View>
      }
    >
      {nodes.map((n) => (
        <PathNode
          key={n.id}
          label={n.label}
          kind={n.kind}
          onPress={n.kind === "current" ? openCurrent : undefined}
          accessibilityLabel={
            n.kind === "current" ? t("path.tap_to_resume") : n.label
          }
        />
      ))}
    </PathColumn>
  );
}
