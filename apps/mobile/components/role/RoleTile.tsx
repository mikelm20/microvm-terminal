// RoleTile. One of the eight department cards in the role-pick grid.
// Visual: warm raised surface, title + one-liner. Haptic confirm happens
// in the parent screen on press.

import { Pressable, Text, View } from "react-native";
import { tokens } from "@learn/shared-tokens";
import type { Department } from "@learn/shared-api/schemas";

type Props = {
  id: Department;
  title: string;
  subtitle: string;
  selected?: boolean;
  onPress: () => void;
};

export function RoleTile({ id, title, subtitle, selected, onPress }: Props) {
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      accessibilityLabel={`${title}. ${subtitle}`}
      testID={`role-tile-${id}`}
      style={({ pressed }) => ({
        flex: 1,
        minHeight: 120,
        padding: 16,
        borderRadius: 20,
        backgroundColor: selected
          ? tokens.color.flame.primary
          : tokens.color.surface.raised,
        borderWidth: 1,
        borderColor: selected
          ? tokens.color.flame.edge
          : tokens.color.surface.divider,
        opacity: pressed ? 0.9 : 1,
      })}
    >
      <View style={{ flex: 1, justifyContent: "space-between" }}>
        <Text
          style={{
            color: selected
              ? tokens.color.surface.sunken
              : tokens.color.ink.primary,
            fontSize: 18,
            fontWeight: "500",
          }}
        >
          {title}
        </Text>
        <Text
          style={{
            color: selected
              ? tokens.color.surface.sunken
              : tokens.color.ink.secondary,
            fontSize: 13,
            lineHeight: 18,
            marginTop: 8,
          }}
        >
          {subtitle}
        </Text>
      </View>
    </Pressable>
  );
}
