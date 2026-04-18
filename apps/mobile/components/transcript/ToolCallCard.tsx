import { View, Text } from "react-native";
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withRepeat,
  withTiming,
  SlideInLeft,
} from "react-native-reanimated";
import { useEffect } from "react";
import { motion, enterTiming } from "../../lib/motion";
import { t } from "../../lib/i18n";
import type { Lang, VoiceKey } from "@learn/shared-voice";
import type { WsEvent as WsEventT, ToolName } from "@learn/shared-api/events";
type ToolCallEvt = Extract<WsEventT, { type: "claude_tool_call" }>;
type ToolResultEvt = Extract<WsEventT, { type: "claude_tool_result" }>;

interface Props {
  call: ToolCallEvt;
  result?: ToolResultEvt;
  lang: Lang;
}

/**
 * Protagonist card. Slides in from the left with a shimmer while pending;
 * settles when the matching tool_result arrives. Copy is derived from
 * lesson.tool_cards.<Tool> in the voice table so nothing is hardcoded.
 */
export function ToolCallCard({ call, result, lang }: Props) {
  const pending = result === undefined;
  const headline = resolveHeadline(call, lang);
  const summary = result?.summary;
  const ok = result?.ok ?? true;

  const progress = useSharedValue(0);
  useEffect(() => {
    if (pending) {
      progress.value = withRepeat(
        withTiming(1, { duration: 900, easing: motion.easing.ambient }),
        -1,
        true,
      );
    } else {
      progress.value = withTiming(0, enterTiming(motion.duration.snap));
    }
  }, [pending, progress]);

  const shimmerStyle = useAnimatedStyle(() => ({
    opacity: 0.18 + progress.value * 0.32,
  }));

  return (
    <Animated.View
      entering={SlideInLeft.duration(motion.duration.base)}
      className="mx-4 my-1.5"
    >
      <View className="overflow-hidden rounded-chip bg-sunken">
        <View className="px-4 py-3">
          <View className="flex-row items-center">
            <View
              className={
                "mr-3 h-2 w-2 rounded-pill " + (pending ? "bg-flame-glow" : ok ? "bg-success" : "bg-attention")
              }
            />
            <Text className="text-ink-secondary text-xs tracking-widest uppercase">
              {labelForTool(call.tool)}
            </Text>
          </View>
          <Text className="text-ink-primary text-base mt-1" numberOfLines={2}>
            {headline}
          </Text>
          {summary ? (
            <Text className="text-ink-tertiary text-xs mt-1 font-mono" numberOfLines={2}>
              {summary}
            </Text>
          ) : null}
        </View>
        {pending ? (
          <Animated.View
            style={shimmerStyle}
            className="h-0.5 bg-flame-primary"
          />
        ) : null}
      </View>
    </Animated.View>
  );
}

function labelForTool(tool: ToolName): string {
  return tool === "Other" ? "TOOL" : tool.toUpperCase();
}

function resolveHeadline(call: ToolCallEvt, lang: Lang): string {
  const key = `lesson.tool_cards.${call.tool}` as VoiceKey;
  const params: Record<string, string> = {};
  const path = call.path ?? call.args["file_path"] ?? call.args["path"] ?? "";
  const command = call.command ?? call.args["command"] ?? "";
  const pattern = call.args["pattern"] ?? "";
  const url = call.args["url"] ?? "";
  const subagent = call.subagent_type ?? "";
  const cmd = call.args["slash"] ?? call.args["command"] ?? "";

  if (path) params["path"] = shortPath(path);
  if (command) params["command"] = shortCommand(command);
  if (pattern) params["pattern"] = pattern;
  if (url) params["url"] = shortUrl(url);
  if (subagent) params["subagent"] = subagent;
  if (cmd) params["command"] = cmd;

  return t(lang, key, params);
}

function shortPath(path: string): string {
  const segments = path.split("/").filter(Boolean);
  if (segments.length <= 2) return path;
  return ".../" + segments.slice(-2).join("/");
}

function shortCommand(cmd: string): string {
  return cmd.length > 56 ? cmd.slice(0, 53) + "..." : cmd;
}

function shortUrl(url: string): string {
  try {
    const u = new URL(url);
    return u.host + u.pathname;
  } catch {
    return url;
  }
}
