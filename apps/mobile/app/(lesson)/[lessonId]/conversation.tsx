import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { View, Text, KeyboardAvoidingView, Platform, Pressable } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useLocalSearchParams, router } from "expo-router";
import { v4 as uuid } from "uuid";
import type { z } from "zod";
import { WsEvent } from "@learn/shared-api/events";
import { Transcript, type OptimisticUserBubble } from "../../../components/transcript/Transcript";
import { Composer } from "../../../components/composer/Composer";
import { api, wizardWsUrl } from "../../../lib/api";
import { WsClient, type WsStatus } from "../../../lib/ws";
import { offlineQueue } from "../../../lib/offline-queue";
import { haptics } from "../../../lib/haptics";
import type { Attachment } from "../../../lib/camera";
import { hardcodedM2Script, runScript } from "../../../lib/event-sim";
import type { Lang } from "@learn/shared-voice";

type WsEventT = z.infer<typeof WsEvent>;

/**
 * The hero screen. Context paragraph at top, streaming transcript in the
 * middle, composer pinned at the bottom thumb zone. When a step_satisfied
 * fires, we push to the Moment screen.
 */
export default function Conversation() {
  const params = useLocalSearchParams<{
    lessonId: string;
    sessionId?: string;
    lang?: string;
    demo?: string; // "1" runs the local simulator
  }>();

  const lessonId = params.lessonId ?? "m2";
  const lang = (params.lang === "en" ? "en" : "es") as Lang;
  const demoMode = !params.sessionId || params.demo === "1";

  const [events, setEvents] = useState<WsEventT[]>([]);
  const [optimistic, setOptimistic] = useState<OptimisticUserBubble[]>([]);
  const [claudeBusy, setClaudeBusy] = useState(false);
  const [sending, setSending] = useState(false);
  const [status, setStatus] = useState<WsStatus>({ kind: "idle" });
  const [accumulatedXp, setAccumulatedXp] = useState(0);

  const wsRef = useRef<WsClient | null>(null);
  const stopSimRef = useRef<(() => void) | null>(null);
  const handledStepsRef = useRef<Set<string>>(new Set());

  const appendEvent = useCallback((ev: WsEventT) => {
    if (ev.type === "claude_busy") {
      setClaudeBusy(ev.busy);
    }
    setEvents((prev) => [...prev, ev]);
  }, []);

  useEffect(() => {
    if (demoMode) {
      // Demo mode turns haptics on immediately so the hero moment is verifiable
      // without having completed three real steps.
      haptics.allowImmediately();
      const script = hardcodedM2Script({ sessionId: "local", lessonId });
      // Fire a simulated initial user prompt so the transcript reads right.
      appendEvent({
        ts: new Date().toISOString(),
        type: "claude_prompt_sent",
        text: lang === "es"
          ? "Claude, mirate la empresa y dime quien es Marta."
          : "Claude, read the company and tell me who Marta is.",
        length: 40,
      } as WsEventT);
      stopSimRef.current = runScript(script, appendEvent);
      return () => {
        stopSimRef.current?.();
      };
    }

    const sessionId = params.sessionId!;
    const client = new WsClient({
      url: wizardWsUrl(sessionId),
      onEvent: appendEvent,
      onStatus: setStatus,
      onReplayRequested: async () => {
        try {
          const tr = await api.getTranscript(sessionId);
          const parsed: WsEventT[] = [];
          for (const raw of tr.events) {
            const res = WsEvent.safeParse(raw);
            if (res.success) parsed.push(res.data);
          }
          setEvents(parsed);
        } catch {
          // transcript unavailable; the live feed will populate.
        }
      },
    });
    wsRef.current = client;
    client.connect();
    return () => {
      client.close();
    };
  }, [demoMode, params.sessionId, lessonId, lang, appendEvent]);

  // Navigate to Moment whenever a step_satisfied arrives (once per step).
  useEffect(() => {
    const latestSatisfied = events.find(
      (e) => e.type === "step_satisfied" && !handledStepsRef.current.has(e.step_id),
    ) as Extract<WsEventT, { type: "step_satisfied" }> | undefined;
    if (!latestSatisfied) return;

    handledStepsRef.current.add(latestSatisfied.step_id);

    const xp = 10;
    setAccumulatedXp((x) => x + xp);

    const headline = headlineFromEvidence(latestSatisfied, lang);

    void haptics.confirm();

    router.push({
      pathname: "/(lesson)/[lessonId]/moment",
      params: {
        lessonId,
        stepId: latestSatisfied.step_id,
        xp: String(xp),
        headline,
        streak: "1",
        first_of_day: "1",
        lang,
      },
    });
  }, [events, lessonId, lang]);

  const contextBody = useMemo(() => contextForLesson(lessonId, lang), [lessonId, lang]);
  const contextTitle = useMemo(() => contextTitleForLesson(lessonId, lang), [lessonId, lang]);

  const handleSubmit = useCallback(
    async (text: string) => {
      const optId = uuid();
      setOptimistic((arr) => [...arr, { kind: "optimistic_user", id: optId, text }]);
      setSending(true);

      if (demoMode) {
        appendEvent({
          ts: new Date().toISOString(),
          type: "claude_prompt_sent",
          text,
          length: text.length,
        } as WsEventT);
        setSending(false);
        return;
      }

      const sessionId = params.sessionId!;
      const idem = uuid();
      const body = { text, client_ts: new Date().toISOString() };

      try {
        await api.submitPrompt(sessionId, body, idem);
        setSending(false);
      } catch {
        // Queue for drain once the network returns; request-id flows server-side.
        await offlineQueue.enqueue({
          endpoint: `/sessions/${sessionId}/prompt`,
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body,
          idempotency_key: idem,
        });
        setSending(false);
      }
    },
    [demoMode, params.sessionId, appendEvent],
  );

  const handleAttach = useCallback(
    async (a: Attachment) => {
      if (demoMode || !params.sessionId) return;
      try {
        const r = await api.attachImage(params.sessionId, {
          uri: a.uri,
          name: a.name,
          mime: a.mime,
        });
        const prefix = lang === "es"
          ? `He recibido la imagen en ${r.inbox_path}. `
          : `I have received the image at ${r.inbox_path}. `;
        await handleSubmit(prefix);
      } catch {
        // silent; learner can retry by tapping camera again
      }
    },
    [demoMode, params.sessionId, lang, handleSubmit],
  );

  return (
    <SafeAreaView edges={["top"]} className="flex-1 bg-substrate">
      <Header lessonId={lessonId} status={status} xp={accumulatedXp} />
      <KeyboardAvoidingView
        behavior={Platform.OS === "ios" ? "padding" : undefined}
        style={{ flex: 1 }}
      >
        <View style={{ flex: 1 }}>
          <Transcript
            events={events}
            optimisticUserBubbles={optimistic}
            lang={lang}
            contextBody={contextBody}
            contextTitle={contextTitle}
          />
        </View>
        <Composer
          lang={lang}
          claudeBusy={claudeBusy}
          sending={sending}
          onSubmit={handleSubmit}
          onAttach={handleAttach}
        />
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

function Header({
  lessonId,
  status,
  xp,
}: {
  lessonId: string;
  status: WsStatus;
  xp: number;
}) {
  const reconnecting = status.kind === "reconnecting";
  return (
    <View className="flex-row items-center justify-between px-4 py-3 border-b border-divider">
      <Pressable
        onPress={() => {
          if (router.canGoBack()) router.back();
        }}
        hitSlop={12}
      >
        <Text className="text-ink-secondary text-sm">
          {"\u2190"}
        </Text>
      </Pressable>
      <Text className="text-ink-primary text-sm">{lessonId}</Text>
      <View className="flex-row items-center">
        {reconnecting ? (
          <Text className="text-ink-tertiary text-xs mr-3">
            {"\u00B7 reconnecting"}
          </Text>
        ) : null}
        <Text className="text-flame-glow text-xs">{xp > 0 ? `+${xp} XP` : ""}</Text>
      </View>
    </View>
  );
}

function contextTitleForLesson(lessonId: string, lang: Lang): string {
  void lessonId;
  return lang === "es" ? "Contexto" : "Context";
}

function contextForLesson(lessonId: string, lang: Lang): string {
  // Placeholder until shared-lessons is wired. Mirrors the m2 opener.
  if (lang === "en") {
    return "You just landed in a fictional company. Claude already opened the relevant file. Ask it who Marta is and watch it read the company notes in front of you.";
  }
  return "Acabas de entrar en una empresa ficticia. Claude ya abrio el archivo que importa. Preguntale quien es Marta y miralo leer las notas de la empresa delante de ti.";
  void lessonId;
}

function headlineFromEvidence(
  ev: Extract<WsEventT, { type: "step_satisfied" }>,
  lang: Lang,
): string {
  const evidence = ev.evidence as { type?: string; tool?: string } | undefined;
  if (evidence && evidence.type === "claude_tool_call" && evidence.tool === "Read") {
    return lang === "es"
      ? "Leiste con Claude tu primer archivo"
      : "You read your first file with Claude";
  }
  return lang === "es" ? "Paso superado" : "Step cleared";
}
