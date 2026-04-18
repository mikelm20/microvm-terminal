import "../global.css";
import { Stack } from "expo-router";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";

/**
 * Minimal root stack. Owned by agent-path in the final merge.
 * Agent-Convo ships this placeholder so the inner (lesson) stack can run
 * before agent-path's scaffold lands.
 */
export default function RootLayout() {
  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <StatusBar style="light" />
        <Stack
          screenOptions={{
            headerShown: false,
            contentStyle: { backgroundColor: "#6e2608" },
          }}
        >
          <Stack.Screen name="index" />
          <Stack.Screen name="(lesson)" />
        </Stack>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}
