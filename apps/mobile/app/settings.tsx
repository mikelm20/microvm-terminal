// Settings screen. Mobile-native list of rows. Each row opens its own
// detail surface (bottom-sheet style) when tapped. Anything that toggles
// in place (haptics, push) uses an inline switch.
//
// Also exposes a small dev panel under __DEV__ so the exit criteria can be
// demonstrated on-device without a backend: clock-forward, complete step,
// hard reset.

import { useMemo, useState } from "react";
import {
  Pressable,
  ScrollView,
  Switch,
  Text,
  View,
} from "react-native";
import { useRouter } from "expo-router";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { tokens } from "@learn/shared-tokens";
import type { Department, Lang } from "@learn/shared-api/schemas";
import { useT, useLang, setSavedLang } from "../lib/i18n";
import {
  useProgress,
  completeStep,
  devJumpDays,
  setDevNowOverride,
  resetAll,
} from "../lib/progress";
import { clearIdentity } from "../lib/identity";
import { patchMe, setDepartment as apiSetDepartment } from "../lib/api";

type Section = "none" | "lang" | "department";

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

function Row({
  label,
  value,
  onPress,
  right,
}: {
  label: string;
  value?: string;
  onPress?: () => void;
  right?: React.ReactNode;
}) {
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole={onPress ? "button" : undefined}
      accessibilityLabel={label}
      style={{
        flexDirection: "row",
        alignItems: "center",
        justifyContent: "space-between",
        paddingVertical: 14,
        paddingHorizontal: 16,
        backgroundColor: tokens.color.surface.raised,
        borderBottomWidth: 1,
        borderBottomColor: tokens.color.surface.divider,
      }}
    >
      <Text style={{ color: tokens.color.ink.primary, fontSize: 15 }}>{label}</Text>
      <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
        {value ? (
          <Text style={{ color: tokens.color.ink.tertiary, fontSize: 14 }}>{value}</Text>
        ) : null}
        {right}
      </View>
    </Pressable>
  );
}

function SectionHeader({ label }: { label: string }) {
  return (
    <Text
      style={{
        color: tokens.color.ink.tertiary,
        fontSize: 12,
        letterSpacing: 2,
        textTransform: "uppercase",
        marginTop: 24,
        marginBottom: 8,
        marginLeft: 16,
      }}
    >
      {label}
    </Text>
  );
}

function BottomSheet({
  visible,
  onClose,
  children,
}: {
  visible: boolean;
  onClose: () => void;
  children: React.ReactNode;
}) {
  if (!visible) return null;
  return (
    <View
      style={{
        position: "absolute",
        left: 0,
        right: 0,
        top: 0,
        bottom: 0,
        backgroundColor: "rgba(0,0,0,0.5)",
        justifyContent: "flex-end",
      }}
    >
      <Pressable onPress={onClose} style={{ flex: 1 }} />
      <View
        style={{
          backgroundColor: tokens.color.surface.raised,
          borderTopLeftRadius: 24,
          borderTopRightRadius: 24,
          paddingVertical: 16,
          paddingHorizontal: 16,
          maxHeight: "70%",
        }}
      >
        {children}
      </View>
    </View>
  );
}

export default function SettingsScreen() {
  const t = useT();
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const progress = useProgress();
  const [lang, setLang] = useLang();
  const [haptics, setHaptics] = useState(true);
  const [push, setPush] = useState(false);
  const [section, setSection] = useState<Section>("none");
  const [department, setLocalDepartment] = useState<Department | null>(null);

  const deptLabel = useMemo(
    () => (department ? t(`role_pick.departments.${department}`) : "-"),
    [department, t],
  );

  const langLabel = lang === "es" ? t("settings.lang_es") : t("settings.lang_en");

  return (
    <View style={{ flex: 1, backgroundColor: tokens.color.surface.substrate }}>
      <ScrollView
        contentContainerStyle={{
          paddingTop: insets.top + 16,
          paddingBottom: insets.bottom + 32,
        }}
      >
        <View
          style={{
            flexDirection: "row",
            alignItems: "center",
            justifyContent: "space-between",
            paddingHorizontal: 16,
            paddingBottom: 8,
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
            {t("settings.title")}
          </Text>
          <Pressable
            onPress={() => router.back()}
            accessibilityRole="button"
            accessibilityLabel={t("settings.done")}
          >
            <Text style={{ color: tokens.color.ink.secondary, fontSize: 15 }}>
              {t("settings.done")}
            </Text>
          </Pressable>
        </View>

        <SectionHeader label={t("settings.section_account")} />
        <Row
          label={t("settings.sign_out")}
          onPress={async () => {
            await clearIdentity();
            router.replace("/auth");
          }}
        />

        <SectionHeader label={t("settings.section_preferences")} />
        <Row
          label={t("settings.lang")}
          value={langLabel}
          onPress={() => setSection("lang")}
        />
        <Row
          label={t("settings.department")}
          value={deptLabel}
          onPress={() => setSection("department")}
        />
        <Row
          label={t("settings.haptics")}
          right={
            <Switch
              value={haptics}
              onValueChange={(v) => {
                setHaptics(v);
                void patchMe({ haptics_enabled: v }).catch(() => {});
              }}
              thumbColor={tokens.color.ink.primary}
              trackColor={{
                true: tokens.color.flame.primary,
                false: tokens.color.surface.divider,
              }}
            />
          }
        />
        <Row
          label={t("settings.push")}
          right={
            <Switch
              value={push}
              onValueChange={(v) => {
                setPush(v);
                void patchMe({ push_enabled: v }).catch(() => {});
              }}
              thumbColor={tokens.color.ink.primary}
              trackColor={{
                true: tokens.color.flame.primary,
                false: tokens.color.surface.divider,
              }}
            />
          }
        />

        <SectionHeader label={t("settings.section_data")} />
        <Row label={t("settings.data_export")} onPress={() => {}} />
        <Row
          label={t("settings.data_delete")}
          onPress={async () => {
            await resetAll();
            await clearIdentity();
            router.replace("/auth");
          }}
        />

        {__DEV__ ? (
          <>
            <SectionHeader label="DEV" />
            <Row
              label="Complete step of current lesson (+10 XP)"
              onPress={() => {
                void completeStep("m2-primera-conversacion", "step-1", 10);
              }}
            />
            <Row
              label="Jump clock +3 days"
              onPress={() => {
                void devJumpDays(3);
              }}
            />
            <Row
              label="Jump clock +14 days"
              onPress={() => {
                void devJumpDays(14);
              }}
            />
            <Row
              label="Jump clock +30 days"
              onPress={() => {
                void devJumpDays(30);
              }}
            />
            <Row
              label="Clear clock override"
              onPress={() => {
                void setDevNowOverride(null);
              }}
            />
            <Row
              label={`Streak: ${progress.streak_days} | Grace: ${progress.grace_tokens} | Modules: ${Object.keys(progress.modules).length}`}
            />
            <Row
              label="Reset all local state"
              onPress={async () => {
                await resetAll();
                await clearIdentity();
                router.replace("/auth");
              }}
            />
          </>
        ) : null}
      </ScrollView>

      <BottomSheet visible={section === "lang"} onClose={() => setSection("none")}>
        {(["es", "en"] as Lang[]).map((l) => (
          <Pressable
            key={l}
            onPress={async () => {
              await setLang(l);
              await setSavedLang(l);
              void patchMe({ lang: l }).catch(() => {});
              setSection("none");
            }}
            style={{ paddingVertical: 14 }}
            accessibilityRole="button"
            accessibilityLabel={l === "es" ? t("settings.lang_es") : t("settings.lang_en")}
          >
            <Text
              style={{
                color:
                  lang === l
                    ? tokens.color.flame.primary
                    : tokens.color.ink.primary,
                fontSize: 16,
              }}
            >
              {l === "es" ? t("settings.lang_es") : t("settings.lang_en")}
            </Text>
          </Pressable>
        ))}
      </BottomSheet>

      <BottomSheet
        visible={section === "department"}
        onClose={() => setSection("none")}
      >
        <ScrollView>
          {DEPARTMENTS.map((d) => (
            <Pressable
              key={d}
              onPress={() => {
                setLocalDepartment(d);
                void apiSetDepartment(d).catch(() => {});
                setSection("none");
              }}
              style={{ paddingVertical: 14 }}
              accessibilityRole="button"
              accessibilityLabel={t(`role_pick.departments.${d}`)}
            >
              <Text
                style={{
                  color:
                    department === d
                      ? tokens.color.flame.primary
                      : tokens.color.ink.primary,
                  fontSize: 16,
                }}
              >
                {t(`role_pick.departments.${d}`)}
              </Text>
            </Pressable>
          ))}
        </ScrollView>
      </BottomSheet>
    </View>
  );
}
