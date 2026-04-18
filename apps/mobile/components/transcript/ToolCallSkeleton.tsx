import { useEffect } from "react";
import { View } from "react-native";
import Animated, {
  useAnimatedStyle,
  useSharedValue,
  withRepeat,
  withTiming,
} from "react-native-reanimated";
import { motion } from "../../lib/motion";

/**
 * Placeholder card that shimmers while a tool call is forming.
 * Appears for the brief window between tool intent detection and
 * the full claude_tool_call event.
 */
export function ToolCallSkeleton() {
  const progress = useSharedValue(0);

  useEffect(() => {
    progress.value = withRepeat(
      withTiming(1, { duration: 1000, easing: motion.easing.ambient }),
      -1,
      true,
    );
  }, [progress]);

  const style = useAnimatedStyle(() => ({
    opacity: 0.5 + progress.value * 0.3,
  }));

  return (
    <View className="mx-4 my-1.5 flex-row items-center">
      <Animated.View
        style={style}
        className="h-12 flex-1 rounded-chip bg-sunken"
      />
    </View>
  );
}
