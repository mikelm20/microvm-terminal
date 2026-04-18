"use client";

import * as React from "react";
import type { Lang, Voice } from "@/lib/i18n";

type VoiceContextValue = {
  lang: Lang;
  voice: Voice;
};

const Ctx = React.createContext<VoiceContextValue | null>(null);

export function VoiceProvider({
  lang,
  voice,
  children,
}: {
  lang: Lang;
  voice: Voice;
  children: React.ReactNode;
}): React.ReactElement {
  const value = React.useMemo(() => ({ lang, voice }), [lang, voice]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useVoice(): VoiceContextValue {
  const v = React.useContext(Ctx);
  if (!v) throw new Error("useVoice must be used inside VoiceProvider");
  return v;
}
