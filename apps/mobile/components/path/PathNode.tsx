// PathNode. A single circle on the vertical Path column.
// The current node breathes and has a warm flame glow behind it.
// Upcoming nodes render at 40% opacity. Locked nodes beyond that do not
// render at all (the parent filters them).

import { useEffect } from "react";
import { Pressable, Text, View } from "react-native";
import Animated, {
  Easing,
  useAnimatedStyle,
  useSharedValue,
  withRepeat,
  withTiming,
  cancelAnimation,
} from "react-native-reanimated";
import { tokens } from "@learn/shared-tokens";

export type NodeKind = "completed" | "current" | "upcoming";

type Props = {
  label: string;
  kind: NodeKind;
  onPress?: () => void;
  accessibilityLabel?: string;
};

export function PathNode({ label, kind, onPress, accessibilityLabel }: Props) {
  const scale = useSharedValue(1);
  const glow = useSharedValue(0);

  useEffect(() => {
    if (kind === "current") {
      scale.value = withRepeat(
        withTiming(1.06, {
          duration: 1800,
          easing: Easing.inOut(Easing.quad),
        }),
        -1,
        true,
      );
      glow.value = withRepeat(
        withTiming(1, { duration: 1800, easing: Easing.inOut(Easing.quad) }),
        -1,
        true,
      );
    } else {
      cancelAnimation(scale);
      cancelAnimation(glow);
      scale.value = 1;
      glow.value = 0;
    }
  }, [kind, scale, glow]);

  const bubbleStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value * (kind === "current" ? 1.4 : 1) }],
  }));

  const glowStyle = useAnimatedStyle(() => ({
    opacity: 0.2 + 0.5 * glow.value,
    transform: [{ scale: 1 + 0.25 * glow.value }],
  }));

  const bgColor =
    kind === "completed"
      ? tokens.color.flame.edge
      : kind === "current"
        ? tokens.color.flame.primary
        : tokens.color.surface.raised;
  const opacity = kind === "upcoming" ? 0.4 : 1;

  return (
    <Pressable
      onPress={onPress}
      disabled={kind === "upcoming"}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel ?? label}
      accessibilityState={{ disabled: kind === "upcoming" }}
      style={{ alignItems: "center", paddingVertical: 16 }}
    >
      <View style={{ alignItems: "center", justifyContent: "center" }}>
        {kind === "current" ? (
          <Animated.View
            pointerEvents="none"
            style={[
              {
                position: "absolute",
                width: 140,
                height: 140,
                borderRadius: 70,
                backgroundColor: tokens.color.flame.glow,
              },
              glowStyle,
            ]}
          />
        ) : null}
        <Animated.View
          style={[
            {
              width: 72,
              height: 72,
              borderRadius: 36,
              alignItems: "center",
              justifyContent: "center",
              backgroundColor: bgColor,
              opacity,
              shadowColor: "#000",
              shadowOffset: { width: 0, height: 12 },
              shadowOpacity: 0.28,
              shadowRadius: 18,
              elevation: 8,
            },
            bubbleStyle,
          ]}
        >
          <Text
            style={{
              color: tokens.color.ink.primary,
              fontSize: 20,
              fontWeight: "500",
            }}
          >
            {kind === "completed" ? "\u2713" : label.charAt(0).toUpperCase()}
          </Text>
        </Animated.View>
      </View>
      <Text
        style={{
          color: tokens.color.ink.secondary,
          marginTop: 12,
          fontSize: 14,
          opacity,
        }}
        numberOfLines={2}
      >
        {label}
      </Text>
    </Pressable>
  );
}
