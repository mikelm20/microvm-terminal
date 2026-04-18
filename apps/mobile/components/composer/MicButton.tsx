import { Pressable, View } from "react-native";
import { haptics } from "../../lib/haptics";
import { IconMic } from "./icons";

interface Props {
  dictating: boolean;
  onStart: () => void;
  onStop: () => void;
  disabled?: boolean;
}

/**
 * Long-press mic. SDK 52 Expo does not ship STT; we rely on the system
 * keyboard's dictation button surfaced via the adjacent TextInput. This
 * button is the affordance: long-press to open the keyboard dictation,
 * release to commit. onStart focuses the input and sets dictating=true.
 */
export function MicButton({ dictating, onStart, onStop, disabled }: Props) {
  async function handleIn() {
    await haptics.tap();
    onStart();
  }
  function handleOut() {
    onStop();
  }

  return (
    <Pressable
      onPressIn={handleIn}
      onPressOut={handleOut}
      disabled={disabled}
      accessibilityRole="button"
      accessibilityLabel="Dictate"
      className={
        "h-11 w-11 items-center justify-center rounded-pill " +
        (dictating ? "bg-flame-primary" : "bg-sunken") +
        (disabled ? " opacity-40" : "")
      }
    >
      <IconMic color={dictating ? "#4a1a06" : "#e6cba3"} />
      {dictating ? (
        <View className="absolute -top-1 -right-1 h-2 w-2 rounded-pill bg-flame-glow" />
      ) : null}
    </Pressable>
  );
}
