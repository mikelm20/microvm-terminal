import { useState } from "react";
import { Pressable, View, Modal, Text } from "react-native";
import { haptics } from "../../lib/haptics";
import { camera, type Attachment } from "../../lib/camera";
import { IconPaperclip } from "./icons";
import { t } from "../../lib/i18n";
import type { Lang } from "@learn/shared-voice";

interface Props {
  lang: Lang;
  disabled?: boolean;
  onPicked: (attachment: Attachment) => void;
}

/**
 * Paperclip affordance. Tap opens a sheet with Camera / Gallery options.
 * No native Alert is used; the sheet is brand-themed and localized.
 */
export function CameraButton({ lang, disabled, onPicked }: Props) {
  const [sheetOpen, setSheetOpen] = useState(false);

  async function openSheet() {
    await haptics.tap();
    setSheetOpen(true);
  }

  async function handleTake() {
    setSheetOpen(false);
    const a = await camera.takePhoto();
    if (a) onPicked(a);
  }

  async function handlePick() {
    setSheetOpen(false);
    const a = await camera.pickFromLibrary();
    if (a) onPicked(a);
  }

  return (
    <>
      <Pressable
        onPress={openSheet}
        disabled={disabled}
        accessibilityRole="button"
        accessibilityLabel={t(lang, "lesson.composer.camera_hint")}
        className={
          "h-11 w-11 items-center justify-center rounded-pill bg-sunken" +
          (disabled ? " opacity-40" : "")
        }
      >
        <IconPaperclip color="#e6cba3" />
      </Pressable>
      <Modal visible={sheetOpen} transparent animationType="slide" onRequestClose={() => setSheetOpen(false)}>
        <Pressable className="flex-1 bg-black/60 justify-end" onPress={() => setSheetOpen(false)}>
          <View className="bg-raised rounded-t-card p-6 pb-10">
            <Text className="text-ink-primary text-base mb-4">
              {t(lang, "lesson.composer.camera_hint")}
            </Text>
            <Pressable onPress={handleTake} className="py-4">
              <Text className="text-ink-primary text-lg">
                {lang === "es" ? "Tomar foto" : "Take photo"}
              </Text>
            </Pressable>
            <View className="h-px bg-divider" />
            <Pressable onPress={handlePick} className="py-4">
              <Text className="text-ink-primary text-lg">
                {lang === "es" ? "Elegir de la galeria" : "Choose from library"}
              </Text>
            </Pressable>
            <View className="h-px bg-divider" />
            <Pressable onPress={() => setSheetOpen(false)} className="py-4">
              <Text className="text-ink-tertiary text-base">
                {lang === "es" ? "Cancelar" : "Cancel"}
              </Text>
            </Pressable>
          </View>
        </Pressable>
      </Modal>
    </>
  );
}
