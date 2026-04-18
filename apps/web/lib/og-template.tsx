import * as React from "react";

type Lang = "es" | "en";

export type OgTemplateProps = {
  lang: Lang;
  eyebrow: string;
  title: string;
  subtitle?: string;
  metaLeft?: string;
  metaRight?: string;
  stat?: { value: string; label: string };
};

/**
 * One branded OG JSX tree reused across profile, module, and certificate
 * dynamic cards. Colors come directly from shared/tokens/tokens.json via the
 * constants below to keep next/og serializable (it cannot evaluate Tailwind).
 */
const BURGUNDY_TOP = "#a8391d";
const BURGUNDY_MID = "#8f2f10";
const BURGUNDY_BOTTOM = "#6e2608";
const CREAM_HI = "#fff6e6";
const CREAM = "#e6cba3";
const EMBER = "#ffc591";
const FLAME = "#ff9b5a";
const RUST = "#e6724a";

export function OgTemplate(props: OgTemplateProps): React.ReactElement {
  return (
    <div
      style={{
        width: "1200px",
        height: "630px",
        display: "flex",
        flexDirection: "column",
        justifyContent: "space-between",
        padding: "64px 80px",
        background: `linear-gradient(180deg, ${BURGUNDY_TOP} 0%, ${BURGUNDY_MID} 60%, ${BURGUNDY_BOTTOM} 100%)`,
        color: CREAM_HI,
        fontFamily: "Inter, system-ui, sans-serif",
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: "16px",
        }}
      >
        <div
          aria-hidden
          style={{
            width: "36px",
            height: "36px",
            borderRadius: "999px",
            background: `linear-gradient(135deg, ${EMBER} 0%, ${FLAME} 50%, ${RUST} 100%)`,
            boxShadow: `0 0 0 6px rgba(255, 197, 145, 0.20)`,
          }}
        />
        <div
          style={{
            fontSize: "22px",
            letterSpacing: "0.18em",
            textTransform: "uppercase",
            color: CREAM,
          }}
        >
          learn.example.com
        </div>
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
        <div
          style={{
            fontSize: "22px",
            letterSpacing: "0.18em",
            textTransform: "uppercase",
            color: EMBER,
          }}
        >
          {props.eyebrow}
        </div>
        <div
          style={{
            fontSize: "72px",
            lineHeight: 1.04,
            letterSpacing: "-0.02em",
            color: CREAM_HI,
            maxWidth: "980px",
          }}
        >
          {props.title}
        </div>
        {props.subtitle ? (
          <div
            style={{
              fontSize: "28px",
              lineHeight: 1.3,
              color: CREAM,
              maxWidth: "980px",
            }}
          >
            {props.subtitle}
          </div>
        ) : null}
      </div>

      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "flex-end",
          borderTop: "1px solid rgba(255, 246, 230, 0.12)",
          paddingTop: "24px",
        }}
      >
        <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
          <div style={{ fontSize: "20px", color: CREAM }}>{props.metaLeft ?? "Platform Engineering"}</div>
          <div style={{ fontSize: "16px", color: CREAM, opacity: 0.7 }}>
            {props.lang === "es" ? "ensenando Claude Code" : "teaching Claude Code"}
          </div>
        </div>

        {props.stat ? (
          <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-end", gap: "4px" }}>
            <div style={{ fontSize: "56px", lineHeight: 1, color: EMBER }}>{props.stat.value}</div>
            <div style={{ fontSize: "18px", letterSpacing: "0.12em", textTransform: "uppercase", color: CREAM }}>
              {props.stat.label}
            </div>
          </div>
        ) : props.metaRight ? (
          <div style={{ fontSize: "20px", color: CREAM }}>{props.metaRight}</div>
        ) : null}
      </div>
    </div>
  );
}

export const OG_WIDTH = 1200;
export const OG_HEIGHT = 630;
