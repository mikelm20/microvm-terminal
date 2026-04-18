/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: "standalone",
  transpilePackages: [
    "@learn/shared-api",
    "@learn/shared-tokens",
    "@learn/shared-voice",
  ],
  images: {
    remotePatterns: [
      {
        protocol: "https",
        hostname: "api.learn.example.com",
      },
      {
        protocol: "https",
        hostname: "learn.example.com",
      },
      {
        protocol: "http",
        hostname: "localhost",
      },
    ],
  },
  async headers() {
    const publicCache = {
      key: "Cache-Control",
      value: "public, s-maxage=60, stale-while-revalidate=600",
    };
    return [
      {
        source: "/:lang/p/:uuid",
        headers: [publicCache],
      },
      {
        source: "/:lang/p/:uuid/m/:lessonId",
        headers: [publicCache],
      },
      {
        source: "/:lang/certificate/:id",
        headers: [publicCache],
      },
      {
        source: "/verify/:id",
        headers: [publicCache],
      },
    ];
  },
};

module.exports = nextConfig;
