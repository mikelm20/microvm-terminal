const path = require("node:path");

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  transpilePackages: [
    "@learn/shared-api",
    "@learn/shared-lessons",
    "@learn/shared-tokens",
    "@learn/shared-voice",
  ],
  output: "standalone",
  outputFileTracingRoot: path.join(__dirname, "../.."),
  experimental: {
    externalDir: true,
  },
};

module.exports = nextConfig;
