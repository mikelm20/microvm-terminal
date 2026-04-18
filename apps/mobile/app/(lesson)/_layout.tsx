import { Stack } from "expo-router";

/**
 * Inner stack for the lesson flow. Conversation is the default; Moment
 * takes over full-bleed when a predicate fires.
 */
export default function LessonLayout() {
  return (
    <Stack
      screenOptions={{
        headerShown: false,
        contentStyle: { backgroundColor: "#6e2608" },
      }}
    >
      <Stack.Screen name="[lessonId]/conversation" />
      <Stack.Screen
        name="[lessonId]/moment"
        options={{
          presentation: "transparentModal",
          animation: "fade",
          gestureEnabled: false,
        }}
      />
    </Stack>
  );
}
