import * as ImagePicker from "expo-image-picker";

/**
 * Camera and gallery picker wrapper. Returns a File-shaped object
 * suitable for multipart upload to POST /sessions/:id/attach.
 */

export interface Attachment {
  uri: string;
  name: string;
  mime: string;
  width?: number;
  height?: number;
}

async function askCameraPerm(): Promise<boolean> {
  const res = await ImagePicker.requestCameraPermissionsAsync();
  return res.granted;
}

async function askLibraryPerm(): Promise<boolean> {
  const res = await ImagePicker.requestMediaLibraryPermissionsAsync();
  return res.granted;
}

function guessName(uri: string): string {
  const parts = uri.split("/");
  return parts[parts.length - 1] ?? `image-${Date.now()}.jpg`;
}

function guessMime(name: string): string {
  if (name.endsWith(".png")) return "image/png";
  if (name.endsWith(".heic")) return "image/heic";
  return "image/jpeg";
}

export const camera = {
  /** Open the camera. Returns null on cancel or denied permission. */
  async takePhoto(): Promise<Attachment | null> {
    if (!(await askCameraPerm())) return null;
    const result = await ImagePicker.launchCameraAsync({
      mediaTypes: ImagePicker.MediaTypeOptions.Images,
      quality: 0.82,
      allowsEditing: false,
      exif: false,
    });
    if (result.canceled || result.assets.length === 0) return null;
    const asset = result.assets[0]!;
    const name = asset.fileName ?? guessName(asset.uri);
    return {
      uri: asset.uri,
      name,
      mime: asset.mimeType ?? guessMime(name),
      width: asset.width,
      height: asset.height,
    };
  },

  /** Open the gallery. Returns null on cancel or denied permission. */
  async pickFromLibrary(): Promise<Attachment | null> {
    if (!(await askLibraryPerm())) return null;
    const result = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ImagePicker.MediaTypeOptions.Images,
      quality: 0.82,
      allowsEditing: false,
      exif: false,
    });
    if (result.canceled || result.assets.length === 0) return null;
    const asset = result.assets[0]!;
    const name = asset.fileName ?? guessName(asset.uri);
    return {
      uri: asset.uri,
      name,
      mime: asset.mimeType ?? guessMime(name),
      width: asset.width,
      height: asset.height,
    };
  },
};
