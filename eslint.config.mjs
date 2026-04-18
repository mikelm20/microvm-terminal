import tseslint from "typescript-eslint";

export default tseslint.config(
  {
    ignores: [
      "**/node_modules/**",
      "**/dist/**",
      "**/build/**",
      "**/.next/**",
      "**/.turbo/**",
      "**/.expo/**",
      "**/coverage/**",
      "control-plane/**",
      "guest-agent/**",
      "vm-image/**",
      "proxy/**",
      "phase-0-spike/**",
      "web/**",
      "infra/**",
      "lessons/**",
      "tests/fixtures/**",
      "**/apitypes.go",
    ],
  },
  ...tseslint.configs.recommended,
  {
    rules: {
      "@typescript-eslint/no-unused-vars": ["warn", { argsIgnorePattern: "^_" }],
      "@typescript-eslint/consistent-type-imports": "warn",
    },
  },
  // Plain .js / .mjs / .cjs files: relax TS-only rules that do not apply.
  {
    files: ["**/*.{js,mjs,cjs}"],
    rules: {
      "@typescript-eslint/no-require-imports": "off",
      "@typescript-eslint/no-var-requires": "off",
      "@typescript-eslint/consistent-type-imports": "off",
    },
  },
);
