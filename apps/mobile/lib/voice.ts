import * as Speech from "expo-speech";
import type { Lang } from "@learn/shared-voice";

/**
 * Lightweight wrapper around expo-speech for TTS output. Input dictation
 * is handled at the platform level by the MicButton component using
 * the native keyboard dictation hook (no Expo SDK speech-to-text exists
 * as of SDK 52; we trigger the system dictation via showVoiceInput hint
 * and rely on onChangeText in the TextInput).
 */

export interface SpeakOptions {
  lang: Lang;
  rate?: number;
  pitch?: number;
}

/** Pick a voice identifier that matches the requested language. */
function localeTag(lang: Lang): string {
  return lang === "es" ? "es-ES" : "en-US";
}

export const speech = {
  /** Speak text out loud. Returns a promise that resolves on done or on stop. */
  async speak(text: string, opts: SpeakOptions): Promise<void> {
    return new Promise((resolve) => {
      Speech.speak(text, {
        language: localeTag(opts.lang),
        rate: opts.rate ?? 1.0,
        pitch: opts.pitch ?? 1.0,
        onDone: () => resolve(),
        onStopped: () => resolve(),
        onError: () => resolve(),
      });
    });
  },

  /** Stop any current speech. */
  stop(): void {
    Speech.stop();
  },

  /** Check if the device is currently speaking. */
  async isSpeaking(): Promise<boolean> {
    try {
      return await Speech.isSpeakingAsync();
    } catch {
      return false;
    }
  },
};

/**
 * Dictation helper. We do not carry an STT engine in-bundle.
 * Live transcription happens in the TextInput itself when the user
 * taps the system keyboard's mic. MicButton acts as an affordance:
 * it focuses the composer and shows the "dictating" state; release
 * flips the state back. Actual text arrives via onChangeText.
 */
export const dictation = {
  startHint(): void {
    // hook for future native module integration
  },
  stopHint(): void {
    // hook for future native module integration
  },
};
