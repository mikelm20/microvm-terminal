// Root layout. Sets up:
//   - the navigation stack (including the lesson group owned by agent-convo)
//   - safe-area + gesture-handler providers
//   - nativewind global stylesheet
//   - first-launch guard: if no identity in secure-store -> /auth
//     if identity but no department -> /role-pick
//     else -> Path (/).
//
// The guard runs AFTER the router mounts so that deep-link navigation
// (e.g. learn://auth/callback) can still claim the session before we bounce
// users around.

import "../global.css";

import { useEffect, useState } from "react";
import { Stack, useRouter, useSegments } from "expo-router";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";
import Constants from "expo-constants";
import { ActivityIndicator, View } from "react-native";
import { tokens } from "@learn/shared-tokens";
import { reattestIdentity, getIdentity } from "../lib/identity";
import { getSavedLang } from "../lib/i18n";
import { hydrateProgress, touchVisit } from "../lib/progress";
import { patchMe } from "../lib/api";

function useBoot(): { ready: boolean; hasIdentity: boolean; department: string | null } {
  const [ready, setReady] = useState(false);
  const [hasIdentity, setHasIdentity] = useState(false);
  const [department, setDepartment] = useState<string | null>(null);

  useEffect(() => {
    let mounted = true;
    (async () => {
      const lang = await getSavedLang();
      const baseUrl =
        (Constants.expoConfig?.extra?.apiBaseUrl as string | undefined) ??
        "http://localhost:8080";
      await hydrateProgress(baseUrl);
      await touchVisit();
      const local = await getIdentity();
      const attested = await reattestIdentity(baseUrl, lang);
      // Best-effort fetch department: for now we rely on the local email
      // claim state plus a no-op patch. Real GET /me lands with agent-convo.
      try {
        const res = await fetch(`${baseUrl}/me`, {
          headers: attested.cookie ? { Cookie: attested.cookie } : {},
        });
        if (res.ok) {
          const me = (await res.json()) as { department: string | null };
          if (mounted) setDepartment(me.department);
        }
      } catch {
        // API not up in dev, no problem.
      }
      if (!mounted) return;
      setHasIdentity(Boolean(local.uuid || attested.uuid));
      setReady(true);
    })();
    return () => {
      mounted = false;
    };
  }, []);

  return { ready, hasIdentity, department };
}

function useFirstLaunchGuard(boot: ReturnType<typeof useBoot>) {
  const segments = useSegments();
  const router = useRouter();
  useEffect(() => {
    if (!boot.ready) return;
    const first = segments[0];
    // Let deep-link callback complete before we redirect.
    if (first === "auth") return;
    if (!boot.hasIdentity && first !== "auth") {
      router.replace("/auth");
      return;
    }
    // If we do not yet know their department, send to role-pick, except
    // when they explicitly chose to go to settings or are already there.
    if (boot.hasIdentity && boot.department === null && first !== "role-pick" && first !== "settings") {
      // Only nudge on cold start, not on every segment change.
      if (!first) router.replace("/role-pick");
    }
  }, [boot.ready, boot.hasIdentity, boot.department, segments, router]);
}

export default function RootLayout() {
  const boot = useBoot();
  useFirstLaunchGuard(boot);

  // Side-effect: whenever Path mounts and we have a department selected
  // elsewhere, ensure the server knows. No-op if API is offline.
  useEffect(() => {
    if (boot.department) {
      void patchMe({}).catch(() => {});
    }
  }, [boot.department]);

  if (!boot.ready) {
    return (
      <View
        style={{
          flex: 1,
          backgroundColor: tokens.color.surface.substrate,
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <ActivityIndicator color={tokens.color.flame.primary} />
      </View>
    );
  }

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <StatusBar style="light" />
        <Stack
          screenOptions={{
            headerShown: false,
            contentStyle: { backgroundColor: tokens.color.surface.substrate },
            animation: "fade",
          }}
        >
          <Stack.Screen name="index" />
          <Stack.Screen name="auth" options={{ animation: "slide_from_bottom" }} />
          <Stack.Screen name="auth/callback" />
          <Stack.Screen name="role-pick" options={{ animation: "slide_from_right" }} />
          <Stack.Screen name="settings" options={{ animation: "slide_from_right" }} />
          <Stack.Screen name="(lesson)/[lessonId]/conversation" />
          <Stack.Screen
            name="(lesson)/[lessonId]/moment"
            options={{ presentation: "fullScreenModal" }}
          />
        </Stack>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}
