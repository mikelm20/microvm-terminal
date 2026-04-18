// Deep-link callback. Fires when the learner taps the magic link on their
// phone. Expo Router matches learn://auth/callback to this file route.
// We parse the token from local-search-params, claim it against the API,
// and navigate into the Path. On failure we show a warm error panel via
// voice strings (no native Alert).

import { useEffect, useState } from "react";
import { Pressable, Text, View } from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { tokens } from "@learn/shared-tokens";
import { useT } from "../../lib/i18n";
import { claimMagicLink } from "../../lib/api";

type Phase = "working" | "ok" | "error";

export default function CallbackScreen() {
  const params = useLocalSearchParams<{ token?: string | string[] }>();
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const t = useT();
  const [phase, setPhase] = useState<Phase>("working");

  useEffect(() => {
    const raw = params.token;
    const token = Array.isArray(raw) ? raw[0] : raw;
    if (!token) {
      setPhase("error");
      return;
    }
    (async () => {
      try {
        await claimMagicLink(token);
        setPhase("ok");
        // Brief flash of "welcome back" then route home.
        setTimeout(() => router.replace("/"), 700);
      } catch {
        setPhase("error");
      }
    })();
  }, [params.token, router]);

  return (
    <View
      style={{
        flex: 1,
        backgroundColor: tokens.color.surface.substrate,
        paddingTop: insets.top + 64,
        paddingBottom: insets.bottom + 32,
        paddingHorizontal: 24,
        justifyContent: "space-between",
      }}
    >
      <View>
        <Text
          style={{
            color: tokens.color.ink.primary,
            fontSize: 28,
            fontWeight: "500",
            letterSpacing: -0.5,
          }}
        >
          {phase === "working"
            ? t("auth.magic_link.sent_title")
            : phase === "ok"
              ? t("auth.claim.welcome_back")
              : t("auth.magic_link.error_generic")}
        </Text>
        <Text
          style={{
            color: tokens.color.ink.secondary,
            fontSize: 16,
            lineHeight: 24,
            marginTop: 12,
          }}
        >
          {phase === "ok" ? t("auth.claim.progress_migrated") : ""}
        </Text>
      </View>
      {phase === "error" ? (
        <Pressable
          onPress={() => router.replace("/auth")}
          accessibilityRole="button"
          accessibilityLabel={t("lesson.error.retry")}
          style={{
            backgroundColor: tokens.color.flame.primary,
            paddingVertical: 16,
            borderRadius: 16,
            alignItems: "center",
          }}
        >
          <Text
            style={{
              color: tokens.color.surface.sunken,
              fontSize: 16,
              fontWeight: "600",
            }}
          >
            {t("lesson.error.retry")}
          </Text>
        </Pressable>
      ) : null}
    </View>
  );
}
