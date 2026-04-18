// StreakRibbon. Lives at the top of the Path screen.
// Shows the streak count, a small grace-token glyph when one or more are
// active, and is tappable (the tap target opens streak history in a future
// iteration, today it is a no-op).

import { Pressable, Text, View } from "react-native";
import { tokens } from "@learn/shared-tokens";
import { useT } from "../../lib/i18n";

type Props = {
  streak: number;
  graceTokens: number;
  onPress?: () => void;
};

export function StreakRibbon({ streak, graceTokens, onPress }: Props) {
  const t = useT();

  const label =
    streak === 0
      ? t("path.streak_zero")
      : streak === 1
        ? t("path.streak_singular", { n: streak })
        : t("path.streak_plural", { n: streak });

  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      accessibilityLabel={label}
      style={{
        flexDirection: "row",
        alignItems: "center",
        alignSelf: "flex-start",
        paddingHorizontal: 12,
        paddingVertical: 6,
        borderRadius: 999,
        backgroundColor: "rgba(255,246,230,0.08)",
        borderWidth: 1,
        borderColor: tokens.color.surface.divider,
      }}
    >
      <View
        style={{
          width: 8,
          height: 8,
          borderRadius: 4,
          marginRight: 8,
          backgroundColor: tokens.color.flame.primary,
        }}
      />
      <Text
        style={{
          color: tokens.color.ink.primary,
          fontSize: 13,
          fontWeight: "500",
        }}
      >
        {label}
      </Text>
      {graceTokens > 0 ? (
        <View
          style={{
            marginLeft: 10,
            flexDirection: "row",
            alignItems: "center",
          }}
        >
          {Array.from({ length: graceTokens }).map((_, i) => (
            <View
              key={i}
              style={{
                width: 6,
                height: 6,
                borderRadius: 3,
                backgroundColor: tokens.color.flame.glow,
                marginLeft: i === 0 ? 0 : 4,
              }}
            />
          ))}
        </View>
      ) : null}
    </Pressable>
  );
}
