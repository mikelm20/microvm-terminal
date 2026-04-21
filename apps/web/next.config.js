/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  transpilePackages: [
    "@learn/shared-api",
    "@learn/shared-lessons",
    "@learn/shared-tokens",
    "@learn/shared-voice",
  ],
  experimental: {
    externalDir: true,
  },
};

module.exports = nextConfig;
