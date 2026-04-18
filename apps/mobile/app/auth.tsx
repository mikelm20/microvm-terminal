// Magic-link entry screen.
//
// Single email field + "send me the link" primary. "Later" secondary mints
// an anonymous identity (already done on first launch) and proceeds to
// role-pick so the user is never blocked from trying the product.

import { useState } from "react";
import {
  KeyboardAvoidingView,
  Platform,
  Pressable,
  Text,
  TextInput,
  View,
} from "react-native";
import { useRouter } from "expo-router";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { tokens } from "@learn/shared-tokens";
import { useT, useLang } from "../lib/i18n";
import { requestMagicLink } from "../lib/api";

type Phase = "idle" | "sending" | "sent" | "error";

export default function AuthScreen() {
  const t = useT();
  const [lang] = useLang();
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const [email, setEmail] = useState("");
  const [phase, setPhase] = useState<Phase>("idle");
  const [errorKey, setErrorKey] = useState<string>("auth.magic_link.error_generic");

  const submit = async () => {
    if (!email.trim()) {
      setErrorKey("auth.magic_link.error_invalid_email");
      setPhase("error");
      return;
    }
    setPhase("sending");
    try {
      await requestMagicLink(email.trim(), lang);
      setPhase("sent");
    } catch {
      setErrorKey("auth.magic_link.error_generic");
      setPhase("error");
    }
  };

  const later = () => {
    router.replace("/role-pick");
  };

  return (
    <KeyboardAvoidingView
      style={{ flex: 1, backgroundColor: tokens.color.surface.substrate }}
      behavior={Platform.OS === "ios" ? "padding" : undefined}
    >
      <View
        style={{
          flex: 1,
          paddingTop: insets.top + 48,
          paddingBottom: insets.bottom + 32,
          paddingHorizontal: 24,
          justifyContent: "space-between",
        }}
      >
        <View>
          <Text
            style={{
              color: tokens.color.ink.primary,
              fontSize: 32,
              fontWeight: "500",
              letterSpacing: -0.5,
            }}
          >
            {phase === "sent" ? t("auth.magic_link.sent_title") : t("auth.magic_link.title")}
          </Text>
          <Text
            style={{
              color: tokens.color.ink.secondary,
              fontSize: 16,
              lineHeight: 24,
              marginTop: 12,
            }}
          >
            {phase === "sent"
              ? t("auth.magic_link.sent_subtitle")
              : t("auth.magic_link.subtitle")}
          </Text>
          {phase !== "sent" ? (
            <View style={{ marginTop: 32 }}>
              <TextInput
                value={email}
                onChangeText={(v) => {
                  setEmail(v);
                  if (phase === "error") setPhase("idle");
                }}
                placeholder={t("auth.magic_link.placeholder")}
                placeholderTextColor={tokens.color.ink.quiet}
                autoComplete="email"
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="email-address"
                inputMode="email"
                textContentType="emailAddress"
                accessibilityLabel={t("auth.magic_link.placeholder")}
                style={{
                  color: tokens.color.ink.primary,
                  fontSize: 18,
                  paddingVertical: 16,
                  paddingHorizontal: 18,
                  borderRadius: 16,
                  backgroundColor: tokens.color.surface.raised,
                  borderWidth: 1,
                  borderColor:
                    phase === "error"
                      ? tokens.color.flame.edge
                      : tokens.color.surface.divider,
                }}
              />
              {phase === "error" ? (
                <Text
                  style={{
                    color: tokens.color.flame.glow,
                    fontSize: 13,
                    marginTop: 8,
                  }}
                >
                  {t(errorKey)}
                </Text>
              ) : null}
            </View>
          ) : null}
        </View>

        <View>
          {phase !== "sent" ? (
            <Pressable
              onPress={submit}
              disabled={phase === "sending"}
              accessibilityRole="button"
              accessibilityLabel={t("auth.magic_link.cta")}
              style={{
                backgroundColor: tokens.color.flame.primary,
                paddingVertical: 16,
                borderRadius: 16,
                alignItems: "center",
                opacity: phase === "sending" ? 0.6 : 1,
              }}
            >
              <Text
                style={{
                  color: tokens.color.surface.sunken,
                  fontSize: 16,
                  fontWeight: "600",
                }}
              >
                {t("auth.magic_link.cta")}
              </Text>
            </Pressable>
          ) : null}
          <Pressable
            onPress={later}
            accessibilityRole="button"
            accessibilityLabel={t("auth.skip")}
            style={{ alignItems: "center", paddingVertical: 16, marginTop: 8 }}
          >
            <Text
              style={{
                color: tokens.color.ink.secondary,
                fontSize: 15,
              }}
            >
              {t("auth.skip")}
            </Text>
          </Pressable>
        </View>
      </View>
    </KeyboardAvoidingView>
  );
}
