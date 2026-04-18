// Role pick. 8 departments as a 2x4 grid on phones, vertical list on very
// narrow screens. Tap selects + haptic + PATCH /me + navigate home.

import { useState } from "react";
import { Pressable, ScrollView, Text, View, useWindowDimensions } from "react-native";
import { useRouter } from "expo-router";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { tokens } from "@learn/shared-tokens";
import type { Department } from "@learn/shared-api/schemas";
import { RoleTile } from "../components/role/RoleTile";
import { useT } from "../lib/i18n";
import { setDepartment } from "../lib/api";
import { haptics } from "../lib/haptics";

const DEPARTMENTS: Department[] = [
  "ventas",
  "marketing",
  "tecnologia",
  "producto",
  "finanzas",
  "rrhh",
  "legal",
  "estrategia",
];

export default function RolePickScreen() {
  const t = useT();
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { width } = useWindowDimensions();
  const [selected, setSelected] = useState<Department | null>(null);
  const [saving, setSaving] = useState(false);

  const columns = width < 360 ? 1 : 2;

  const onPick = async (id: Department) => {
    setSelected(id);
    void haptics.confirm();
    setSaving(true);
    // Fire and forget: the UI does not block on the server acknowledging.
    void setDepartment(id).catch(() => {});
    setTimeout(() => {
      setSaving(false);
      router.replace("/");
    }, 220);
  };

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: tokens.color.surface.substrate }}
      contentContainerStyle={{
        paddingTop: insets.top + 32,
        paddingBottom: insets.bottom + 24,
        paddingHorizontal: 24,
      }}
    >
      <Text
        style={{
          color: tokens.color.ink.primary,
          fontSize: 28,
          fontWeight: "500",
          letterSpacing: -0.5,
        }}
      >
        {t("role_pick.title")}
      </Text>
      <Text
        style={{
          color: tokens.color.ink.secondary,
          fontSize: 16,
          lineHeight: 24,
          marginTop: 8,
        }}
      >
        {t("role_pick.subtitle")}
      </Text>

      <View
        style={{
          marginTop: 28,
          flexDirection: columns === 1 ? "column" : "row",
          flexWrap: columns === 1 ? "nowrap" : "wrap",
          gap: 12,
        }}
      >
        {DEPARTMENTS.map((id) => (
          <View
            key={id}
            style={{
              width:
                columns === 1
                  ? undefined
                  : "48%" as unknown as number,
            }}
          >
            <RoleTile
              id={id}
              title={t(`role_pick.departments.${id}`)}
              subtitle={t(`role_pick.taglines.${id}`)}
              selected={selected === id}
              onPress={() => {
                if (!saving) void onPick(id);
              }}
            />
          </View>
        ))}
      </View>

      <Pressable
        onPress={() => router.replace("/")}
        accessibilityRole="button"
        accessibilityLabel={t("role_pick.skip")}
        style={{ alignItems: "center", paddingVertical: 20, marginTop: 16 }}
      >
        <Text
          style={{
            color: tokens.color.ink.tertiary,
            fontSize: 14,
          }}
        >
          {t("role_pick.skip")}
        </Text>
      </Pressable>
    </ScrollView>
  );
}
