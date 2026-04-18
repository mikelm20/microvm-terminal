import { View, Text } from "react-native";
import Animated, { FadeIn } from "react-native-reanimated";
import { motion } from "../../lib/motion";

interface Props {
  text: string;
  /** True while the local optimistic copy has not yet been echoed by server. */
  optimistic?: boolean;
}

/**
 * Right-aligned warm-cream bubble. Opacity drops while optimistic so the
 * learner sees the bubble lock in once the server confirms.
 */
export function UserBubble({ text, optimistic }: Props) {
  return (
    <Animated.View
      entering={FadeIn.duration(motion.duration.snap)}
      style={{ opacity: optimistic ? 0.55 : 1 }}
      className="items-end my-1.5 px-4"
    >
      <View className="max-w-[82%] rounded-card bg-warm px-4 py-3">
        <Text className="text-deep text-base leading-6">{text}</Text>
      </View>
    </Animated.View>
  );
}
