import { useEffect, useMemo, useRef } from "react";
import { ScrollView, View, Text } from "react-native";
import type { WsEvent as WsEventT } from "@learn/shared-api/events";
import { UserBubble } from "./UserBubble";
import { ClaudeBubble } from "./ClaudeBubble";
import { ToolCallCard } from "./ToolCallCard";
import { t } from "../../lib/i18n";
import type { Lang } from "@learn/shared-voice";

export interface OptimisticUserBubble {
  kind: "optimistic_user";
  id: string;
  text: string;
}

export interface TranscriptProps {
  events: WsEventT[];
  optimisticUserBubbles: OptimisticUserBubble[];
  lang: Lang;
  /** Paragraph above the transcript (lesson.steps[i].body). Rendered at top. */
  contextBody: string;
  contextTitle?: string;
}

/**
 * Collapses the event stream into a chronological list of renderables:
 *   - context paragraph at the very top
 *   - user bubbles (server-echoed user messages + optimistic ones still in flight)
 *   - tool-call cards (joined with their matching tool_result)
 *   - claude bubbles (assistant messages, streaming or complete)
 */
export function Transcript({
  events,
  optimisticUserBubbles,
  lang,
  contextBody,
  contextTitle,
}: TranscriptProps) {
  const renderables = useMemo(
    () => collapse(events, optimisticUserBubbles),
    [events, optimisticUserBubbles],
  );

  const scrollRef = useRef<ScrollView | null>(null);
  useEffect(() => {
    // Auto-scroll to tail on every append.
    const id = setTimeout(() => {
      scrollRef.current?.scrollToEnd({ animated: true });
    }, 60);
    return () => clearTimeout(id);
  }, [renderables.length]);

  return (
    <ScrollView
      ref={scrollRef}
      keyboardShouldPersistTaps="handled"
      showsVerticalScrollIndicator={false}
      contentContainerStyle={{ paddingBottom: 24 }}
    >
      <View className="px-4 pt-2 pb-4">
        {contextTitle ? (
          <Text className="text-ink-secondary text-xs tracking-widest uppercase mb-2">
            {contextTitle}
          </Text>
        ) : null}
        <Text className="text-ink-primary text-base leading-6">{contextBody}</Text>
        <Text className="text-ink-tertiary text-xs mt-3">
          {t(lang, "lesson.context.reading_hint")}
        </Text>
      </View>

      {renderables.length === 0 ? null : (
        <View>
          {renderables.map((r) => {
            switch (r.kind) {
              case "user":
                return <UserBubble key={r.id} text={r.text} optimistic={r.optimistic} />;
              case "claude":
                return (
                  <ClaudeBubble
                    key={r.id}
                    turnId={r.id}
                    streamedText={r.streamedText}
                    finalText={r.finalText}
                    streaming={r.streaming}
                    lang={lang}
                  />
                );
              case "tool":
                return <ToolCallCard key={r.id} call={r.call} result={r.result} lang={lang} />;
              default:
                return null;
            }
          })}
        </View>
      )}
    </ScrollView>
  );
}

type Renderable =
  | { kind: "user"; id: string; text: string; optimistic: boolean; ts: string }
  | {
      kind: "claude";
      id: string;
      streamedText?: string;
      finalText?: string;
      streaming: boolean;
      ts: string;
    }
  | {
      kind: "tool";
      id: string;
      call: Extract<WsEventT, { type: "claude_tool_call" }>;
      result?: Extract<WsEventT, { type: "claude_tool_result" }>;
      ts: string;
    };

/**
 * Walk the event stream once and produce the ordered render list.
 * Tool calls are paired with their results by call_id. Streamed tokens
 * accumulate into a single claude bubble keyed by turn_id. User
 * messages arrive as claude_message role=user; optimistic ones carry
 * client-only ids until the server echoes.
 */
function collapse(
  events: WsEventT[],
  optimistic: OptimisticUserBubble[],
): Renderable[] {
  const items: Renderable[] = [];
  const toolByCallId = new Map<string, number>(); // index into items
  const claudeByTurn = new Map<string, number>(); // index into items
  let claudeBusy = false;

  const seenUserTexts: string[] = [];

  for (const ev of events) {
    switch (ev.type) {
      case "claude_busy":
        claudeBusy = ev.busy;
        break;

      case "claude_tool_call": {
        const item: Renderable = {
          kind: "tool",
          id: ev.call_id,
          call: ev,
          ts: ev.ts,
        };
        toolByCallId.set(ev.call_id, items.length);
        items.push(item);
        break;
      }

      case "claude_tool_result": {
        const idx = toolByCallId.get(ev.call_id);
        if (idx !== undefined) {
          const existing = items[idx];
          if (existing && existing.kind === "tool") {
            items[idx] = { ...existing, result: ev };
          }
        }
        break;
      }

      case "claude_token_streamed": {
        const idx = claudeByTurn.get(ev.turn_id);
        if (idx === undefined) {
          const item: Renderable = {
            kind: "claude",
            id: ev.turn_id,
            streamedText: ev.delta,
            streaming: true,
            ts: ev.ts,
          };
          claudeByTurn.set(ev.turn_id, items.length);
          items.push(item);
        } else {
          const existing = items[idx];
          if (existing && existing.kind === "claude") {
            items[idx] = {
              ...existing,
              streamedText: (existing.streamedText ?? "") + ev.delta,
              streaming: true,
            };
          }
        }
        break;
      }

      case "claude_message": {
        if (ev.role === "user") {
          seenUserTexts.push(ev.text);
          items.push({
            kind: "user",
            id: ev.turn_id,
            text: ev.text,
            optimistic: false,
            ts: ev.ts,
          });
        } else if (ev.role === "assistant") {
          const idx = claudeByTurn.get(ev.turn_id);
          if (idx === undefined) {
            claudeByTurn.set(ev.turn_id, items.length);
            items.push({
              kind: "claude",
              id: ev.turn_id,
              finalText: ev.text,
              streaming: false,
              ts: ev.ts,
            });
          } else {
            const existing = items[idx];
            if (existing && existing.kind === "claude") {
              items[idx] = { ...existing, finalText: ev.text, streaming: false };
            }
          }
        }
        break;
      }

      case "claude_prompt_sent": {
        seenUserTexts.push(ev.text);
        items.push({
          kind: "user",
          id: `prompt-${ev.ts}`,
          text: ev.text,
          optimistic: false,
          ts: ev.ts,
        });
        break;
      }

      default:
        break;
    }
  }

  // Append optimistic bubbles the server has not yet echoed.
  for (const o of optimistic) {
    if (seenUserTexts.includes(o.text)) continue;
    items.push({
      kind: "user",
      id: `opt-${o.id}`,
      text: o.text,
      optimistic: true,
      ts: new Date().toISOString(),
    });
  }

  void claudeBusy;
  return items;
}
