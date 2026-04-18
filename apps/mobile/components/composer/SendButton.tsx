import { Pressable, View } from "react-native";
import { IconSend, IconSpinner } from "./icons";
import { haptics } from "../../lib/haptics";

export type SendState = "ready" | "sending" | "disabled" | "busy";

interface Props {
  state: SendState;
  onPress: () => void;
}

/**
 * Primary composer action. State informs the glyph and disabled flag.
 * `busy` is Claude-is-working, `sending` is the local request in flight.
 */
export function SendButton({ state, onPress }: Props) {
  const disabled = state === "disabled" || state === "busy";
  const working = state === "sending" || state === "busy";

  async function handlePress() {
    if (disabled) return;
    await haptics.tap();
    onPress();
  }

  return (
    <Pressable
      onPress={handlePress}
      disabled={disabled}
      accessibilityRole="button"
      accessibilityLabel="Send"
      className={
        "h-11 w-11 items-center justify-center rounded-pill " +
        (disabled ? "bg-sunken" : "bg-flame-primary") +
        (disabled ? " opacity-50" : "")
      }
    >
      <View>
        {working ? <IconSpinner color="#4a1a06" /> : <IconSend color="#4a1a06" />}
      </View>
    </Pressable>
  );
}
