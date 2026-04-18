import { useEffect, useState } from "react";
import { View, Text, Pressable } from "react-native";
import Animated, { FadeIn } from "react-native-reanimated";
import { motion } from "../../lib/motion";
import { speech } from "../../lib/voice";
import type { Lang } from "@learn/shared-voice";

interface Props {
  /** Stable id tying incremental stream chunks to a single bubble. */
  turnId: string;
  /** Final text if we have it; otherwise stream deltas drive the view. */
  finalText?: string;
  /** Streamed text accumulated from claude_token_streamed deltas. */
  streamedText?: string;
  /** True while tokens are still arriving. */
  streaming?: boolean;
  lang: Lang;
}

/**
 * Left-aligned raised card. Renders a blinking caret while streaming so
 * the learner feels the typing in the moment. Tap the headphone glyph
 * to hear Claude speak the message out loud.
 */
export function ClaudeBubble({ turnId, finalText, streamedText, streaming, lang }: Props) {
  void turnId;
  const visibleText = finalText ?? streamedText ?? "";
  const [caretOn, setCaretOn] = useState(true);

  useEffect(() => {
    if (!streaming) return;
    const id = setInterval(() => setCaretOn((c) => !c), 520);
    return () => clearInterval(id);
  }, [streaming]);

  async function handleSpeak() {
    if (!visibleText) return;
    await speech.speak(visibleText, { lang });
  }

  return (
    <Animated.View
      entering={FadeIn.duration(motion.duration.base)}
      className="items-start my-1.5 px-4"
    >
      <View className="max-w-[86%] rounded-card bg-raised px-4 py-3">
        <Text className="text-ink-primary text-base leading-6">
          {visibleText}
          {streaming ? (
            <Text className="text-flame-glow">{caretOn ? " \u258A" : " \u00A0"}</Text>
          ) : null}
        </Text>
        {visibleText && !streaming ? (
          <Pressable
            onPress={handleSpeak}
            hitSlop={12}
            className="mt-2 self-start rounded-pill px-2 py-1"
            accessibilityLabel="Play aloud"
          >
            <Text className="text-ink-secondary text-xs">{lang === "es" ? "Oir" : "Listen"}</Text>
          </Pressable>
        ) : null}
      </View>
    </Animated.View>
  );
}
