// ReturnCard. Renders above the Path when a learner comes back after a
// gap. Three buckets:
//   short  (2-7 days):  warm "keep going" CTA
//   medium (7-21 days): offer a review first
//   long   (21+ days):  low-pressure, "ease back in"
// When a grace token was just consumed, we show the grace line instead of
// the bucket title so the learner never feels scolded.

import { Pressable, Text, View } from "react-native";
import { tokens } from "@learn/shared-tokens";
import { useT } from "../../lib/i18n";
import type { ReturnBucket } from "../../lib/progress";

type Props = {
  bucket: ReturnBucket;
  days: number;
  graceUsed?: boolean;
  streakBroken?: boolean;
  onPrimary: () => void;
  onSecondary?: () => void;
};

export function ReturnCard({
  bucket,
  days,
  graceUsed,
  streakBroken,
  onPrimary,
  onSecondary,
}: Props) {
  const t = useT();
  if (bucket === "none") return null;

  const title =
    bucket === "short"
      ? t("return.short_title", { days })
      : bucket === "medium"
        ? t("return.medium_title")
        : t("return.long_title");

  const sub =
    bucket === "medium"
      ? t("return.medium_sub")
      : bucket === "long"
        ? t("return.long_sub")
        : null;

  const primaryLabel =
    bucket === "short"
      ? t("return.short_cta")
      : bucket === "medium"
        ? t("return.medium_continue")
        : t("return.long_cta");

  const secondaryLabel =
    bucket === "medium" ? t("return.medium_review") : null;

  const topline = graceUsed
    ? t("return.grace_used")
    : streakBroken
      ? t("return.streak_broken")
      : null;

  return (
    <View
      style={{
        backgroundColor: tokens.color.surface.raised,
        borderRadius: 24,
        padding: 20,
        borderWidth: 1,
        borderColor: tokens.color.surface.divider,
      }}
      accessibilityRole="summary"
    >
      {topline ? (
        <Text
          style={{
            color: tokens.color.ink.tertiary,
            fontSize: 12,
            letterSpacing: 2,
            textTransform: "uppercase",
            marginBottom: 8,
          }}
        >
          {topline}
        </Text>
      ) : null}
      <Text
        style={{
          color: tokens.color.ink.primary,
          fontSize: 22,
          fontWeight: "500",
        }}
      >
        {title}
      </Text>
      {sub ? (
        <Text
          style={{
            color: tokens.color.ink.secondary,
            fontSize: 15,
            marginTop: 8,
            lineHeight: 22,
          }}
        >
          {sub}
        </Text>
      ) : null}
      <Pressable
        onPress={onPrimary}
        accessibilityRole="button"
        accessibilityLabel={primaryLabel}
        style={{
          marginTop: 16,
          backgroundColor: tokens.color.flame.primary,
          paddingVertical: 14,
          borderRadius: 16,
          alignItems: "center",
        }}
      >
        <Text
          style={{
            color: tokens.color.surface.sunken,
            fontSize: 16,
            fontWeight: "600",
          }}
        >
          {primaryLabel}
        </Text>
      </Pressable>
      {secondaryLabel && onSecondary ? (
        <Pressable
          onPress={onSecondary}
          accessibilityRole="button"
          accessibilityLabel={secondaryLabel}
          style={{ marginTop: 10, alignItems: "center", paddingVertical: 10 }}
        >
          <Text
            style={{
              color: tokens.color.ink.secondary,
              fontSize: 14,
            }}
          >
            {secondaryLabel}
          </Text>
        </Pressable>
      ) : null}
    </View>
  );
}
