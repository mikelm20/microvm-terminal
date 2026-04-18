// Vertical column of lesson nodes. Safe-area aware.
// Spacing is generous and deliberate: the page is meant to feel calm, not
// like an achievement grid.

import { ReactNode } from "react";
import { ScrollView, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { tokens } from "@learn/shared-tokens";

type Props = {
  header?: ReactNode;
  children: ReactNode;
};

export function PathColumn({ header, children }: Props) {
  const insets = useSafeAreaInsets();
  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: tokens.color.surface.substrate }}
      contentContainerStyle={{
        paddingTop: insets.top + 16,
        paddingBottom: insets.bottom + 48,
        paddingHorizontal: 24,
      }}
      showsVerticalScrollIndicator={false}
    >
      {header ? <View style={{ marginBottom: 16 }}>{header}</View> : null}
      <View style={{ alignItems: "center", gap: 28 }}>{children}</View>
    </ScrollView>
  );
}
