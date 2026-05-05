import type { NextConfig } from "next";

const noStoreHeaders = [
  {
    key: "Cache-Control",
    value: "private, no-cache, no-store, max-age=0, must-revalidate",
  },
];

const nextConfig: NextConfig = {
  reactStrictMode: true,
  output: "standalone",
  async headers() {
    return ["/", "/login", "/setup", "/dashboard", "/dashboard/:path*"].map(
      (source) => ({
        source,
        headers: noStoreHeaders,
      }),
    );
  },
};

export default nextConfig;
