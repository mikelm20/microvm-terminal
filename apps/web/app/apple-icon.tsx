import { ImageResponse } from "next/og";

export const runtime = "edge";
export const size = { width: 180, height: 180 };
export const contentType = "image/png";

export default function AppleIcon(): ImageResponse {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background:
            "linear-gradient(180deg, #a8391d 0%, #8f2f10 60%, #6e2608 100%)",
        }}
      >
        <div
          style={{
            width: "96px",
            height: "96px",
            borderRadius: "9999px",
            background:
              "linear-gradient(135deg, #ffc591 0%, #ff9b5a 50%, #e6724a 100%)",
            boxShadow: "0 0 0 8px rgba(255, 197, 145, 0.18)",
          }}
        />
      </div>
    ),
    { ...size },
  );
}
