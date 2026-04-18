import Svg, { Path, Rect, Circle } from "react-native-svg";

interface IconProps {
  size?: number;
  color?: string;
}

/**
 * Minimal icon set for the composer. All icons derive their color from the
 * surrounding text color; default size 22. No external icon font needed.
 */

export function IconMic({ size = 22, color = "currentColor" }: IconProps) {
  return (
    <Svg width={size} height={size} viewBox="0 0 24 24" fill="none">
      <Path
        d="M12 14.5a3 3 0 0 0 3-3v-5a3 3 0 0 0-6 0v5a3 3 0 0 0 3 3Z"
        stroke={color}
        strokeWidth={1.6}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <Path
        d="M5.5 11.5v0a6.5 6.5 0 0 0 13 0v0"
        stroke={color}
        strokeWidth={1.6}
        strokeLinecap="round"
      />
      <Path d="M12 18v3" stroke={color} strokeWidth={1.6} strokeLinecap="round" />
    </Svg>
  );
}

export function IconSend({ size = 22, color = "currentColor" }: IconProps) {
  return (
    <Svg width={size} height={size} viewBox="0 0 24 24" fill="none">
      <Path
        d="M4 20 21 12 4 4l3.5 8L4 20Z"
        stroke={color}
        strokeWidth={1.6}
        strokeLinejoin="round"
        fill="none"
      />
    </Svg>
  );
}

export function IconPaperclip({ size = 22, color = "currentColor" }: IconProps) {
  return (
    <Svg width={size} height={size} viewBox="0 0 24 24" fill="none">
      <Path
        d="M20 11.5 12.5 19a4.5 4.5 0 0 1-6.36-6.36L14 5a3 3 0 0 1 4.24 4.24L10.5 17"
        stroke={color}
        strokeWidth={1.6}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </Svg>
  );
}

export function IconSpinner({ size = 18, color = "currentColor" }: IconProps) {
  return (
    <Svg width={size} height={size} viewBox="0 0 24 24" fill="none">
      <Circle cx="12" cy="12" r="9" stroke={color} strokeOpacity={0.2} strokeWidth={2} />
      <Path d="M21 12a9 9 0 0 0-9-9" stroke={color} strokeWidth={2} strokeLinecap="round" />
    </Svg>
  );
}

export function IconCheck({ size = 22, color = "currentColor" }: IconProps) {
  return (
    <Svg width={size} height={size} viewBox="0 0 24 24" fill="none">
      <Path
        d="m5 12 5 5 9-10"
        stroke={color}
        strokeWidth={2}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </Svg>
  );
}

export function IconDot({ size = 6, color = "currentColor" }: IconProps) {
  return (
    <Svg width={size} height={size} viewBox="0 0 8 8">
      <Rect x="0" y="0" width="8" height="8" rx="4" fill={color} />
    </Svg>
  );
}
