/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // linux/amd64 builds on the arm64 Mac runner use QEMU; static page workers
  // routinely exceed the default 60s and get SIGTERM'd (see v1.0.225 publish).
  staticPageGenerationTimeout: 600,
  experimental: {
    // Keep generation serial under emulation — parallel workers amplify timeouts.
    cpus: 1,
    workerThreads: false,
  },
};

export default nextConfig;
