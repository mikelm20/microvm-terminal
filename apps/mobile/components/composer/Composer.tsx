import { useRef, useState } from "react";
import { View, TextInput, Text, Keyboard } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { MicButton } from "./MicButton";
import { CameraButton } from "./CameraButton";
import { SendButton } from "./SendButton";
import type { Attachment } from "../../lib/camera";
import { t } from "../../lib/i18n";
import type { Lang } from "@learn/shared-voice";

export interface ComposerProps {
  lang: Lang;
  /** True while Claude is working on the previous turn. Send disabled. */
  claudeBusy: boolean;
  /** True while a request we fired has not yet been acked. */
  sending: boolean;
  /** Called with the final text to submit. */
  onSubmit: (text: string) => void;
  /** Called with a picked attachment; parent orchestrates the upload. */
  onAttach?: (a: Attachment) => void;
  /** Default mode (voice-first lessons start the mic on). */
  voiceFirst?: boolean;
  /** Step requires a camera; paperclip pulses to draw attention. */
  requiresCamera?: boolean;
}

/**
 * 88 dp tall (before safe-area inset). Pinned to the bottom. Left paperclip,
 * middle expanding text, right mic + send. Disabled visuals when claude_busy.
 */
export function Composer({
  lang,
  claudeBusy,
  sending,
  onSubmit,
  onAttach,
  voiceFirst,
  requiresCamera,
}: ComposerProps) {
  const inset = useSafeAreaInsets();
  const [value, setValue] = useState("");
  const [dictating, setDictating] = useState(false);
  const inputRef = useRef<TextInput | null>(null);

  const ready = value.trim().length > 0 && !claudeBusy && !sending;
  const sendState = claudeBusy
    ? ("busy" as const)
    : sending
      ? ("sending" as const)
      : ready
        ? ("ready" as const)
        : ("disabled" as const);

  function handleSubmit() {
    const text = value.trim();
    if (!text) return;
    onSubmit(text);
    setValue("");
    Keyboard.dismiss();
  }

  function handleMicStart() {
    setDictating(true);
    inputRef.current?.focus();
  }
  function handleMicStop() {
    setDictating(false);
  }

  function handleAttach(a: Attachment) {
    onAttach?.(a);
  }

  const placeholder = voiceFirst
    ? t(lang, "lesson.composer.placeholder_voice")
    : t(lang, "lesson.composer.placeholder");

  const statusLabel = dictating
    ? t(lang, "lesson.composer.dictating")
    : claudeBusy
      ? t(lang, "lesson.composer.busy")
      : sending
        ? t(lang, "lesson.composer.sending")
        : null;

  void requiresCamera;

  return (
    <View
      className="bg-substrate border-t border-divider"
      style={{ paddingBottom: inset.bottom }}
    >
      {statusLabel ? (
        <View className="px-4 pt-2">
          <Text className="text-ink-tertiary text-xs">{statusLabel}</Text>
        </View>
      ) : null}
      <View className="flex-row items-end px-3 py-3 gap-2">
        <CameraButton lang={lang} onPicked={handleAttach} disabled={claudeBusy || sending} />
        <View className="flex-1 min-h-11 max-h-40 rounded-card bg-sunken px-3 py-2 justify-center">
          <TextInput
            ref={inputRef}
            value={value}
            onChangeText={setValue}
            placeholder={placeholder}
            placeholderTextColor="#a89070"
            multiline
            style={{
              color: "#fff6e6",
              fontSize: 16,
              lineHeight: 22,
              minHeight: 28,
              paddingTop: 0,
              paddingBottom: 0,
            }}
            editable={!claudeBusy && !sending}
            blurOnSubmit={false}
            returnKeyType="default"
            accessibilityLabel={placeholder}
          />
        </View>
        <MicButton
          dictating={dictating}
          onStart={handleMicStart}
          onStop={handleMicStop}
          disabled={claudeBusy || sending}
        />
        <SendButton state={sendState} onPress={handleSubmit} />
      </View>
    </View>
  );
}
