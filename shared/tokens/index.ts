export { default as tokens } from "./tokens.json" with { type: "json" };
export type Tokens = typeof import("./tokens.json");
