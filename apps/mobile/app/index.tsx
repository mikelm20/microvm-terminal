import { View, Text, Pressable } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";

/**
 * Placeholder home. Agent-Path replaces this with the real Path.
 * A single CTA navigates into the hero (lesson m2 conversation).
 */
export default function Home() {
  return (
    <SafeAreaView className="flex-1 bg-substrate">
      <View className="flex-1 items-center justify-center px-6">
        <Text className="text-ink-primary text-2xl mb-4">learn</Text>
        <Text className="text-ink-secondary text-base mb-12 text-center">
          Agent-Path owns this screen in the final merge.
        </Text>
        <Pressable
          onPress={() => router.push("/(lesson)/m2/conversation")}
          className="bg-flame-primary rounded-pill px-8 py-4"
        >
          <Text className="text-deep text-base">Open m2 conversation</Text>
        </Pressable>
      </View>
    </SafeAreaView>
  );
}
