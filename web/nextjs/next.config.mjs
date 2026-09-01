const apiBase = (process.env.NEXT_PUBLIC_API_URL ?? "http://127.0.0.1:8080").replace(
  /\/$/,
  ""
);

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  async rewrites() {
    return [
      // Static marketing page in public/landing; its asset paths are absolute
      // (/landing/...) so it serves correctly with or without a trailing slash.
      { source: "/landing", destination: "/landing/index.html" },
      // Article markdown uses /api/v1/images/file/… — without this, a plain
      // <img> on the Next origin 404s and the prompt shows as alt text.
      {
        source: "/api/v1/images/file/:path*",
        destination: `${apiBase}/api/v1/images/file/:path*`,
      },
    ];
  },
  images: {
    remotePatterns: [
      {
        protocol: "http",
        hostname: "localhost",
        port: "9000",
        pathname: "/**",
      },
      {
        protocol: "http",
        hostname: "minio",
        port: "9000",
        pathname: "/**",
      },
    ],
  },
  async redirects() {
    return [
      { source: "/dashboard", destination: "/panel/dashboard", permanent: false },
      { source: "/metrics", destination: "/panel/dashboard", permanent: false },
      { source: "/agent-board", destination: "/panel/task-board", permanent: false },
      { source: "/tasks", destination: "/panel/task-board", permanent: false },
      { source: "/task-manager", destination: "/panel/task-manager", permanent: false },
      { source: "/projects", destination: "/panel/projects", permanent: false },
      {
        source: "/projects/:id",
        destination: "/panel/projects?project=:id",
        permanent: false,
      },
      { source: "/agents", destination: "/panel/task-manager", permanent: false },
      {
        source: "/agents/pentest",
        destination: "/panel/task-manager?agent=pentest",
        permanent: false,
      },
      {
        source: "/agents/review",
        destination: "/panel/task-manager?agent=review",
        permanent: false,
      },
      {
        source: "/agents/mail",
        destination: "/panel/task-manager?agent=mail",
        permanent: false,
      },
      // /agents/:id and /agents/:id/knowledge stay routable — the rich agent
      // profile (edit, knowledge, skills, metrics) is linked from Task Manager.
      {
        source: "/articles",
        destination: "/panel/task-manager?agent=articles",
        permanent: false,
      },
      // Keep /articles/:runId for article detail (linked from Articles list)
      { source: "/artifacts", destination: "/panel/artifacts", permanent: false },
      {
        source: "/images",
        destination: "/panel/task-manager?agent=images",
        permanent: false,
      },
      { source: "/sessions", destination: "/panel/sessions", permanent: false },
      { source: "/scheduler", destination: "/panel/scheduler", permanent: false },
      { source: "/sprints", destination: "/panel/sprints", permanent: false },
      { source: "/workflows", destination: "/panel/workflows", permanent: false },
      { source: "/org-builder", destination: "/panel/org-builder", permanent: false },
      { source: "/marketplace", destination: "/panel/marketplace", permanent: false },
      { source: "/plugins", destination: "/panel/plugins-skills", permanent: false },
      { source: "/skills", destination: "/panel/plugins-skills", permanent: false },
      {
        source: "/llm-providers",
        destination: "/panel/llm-providers",
        permanent: false,
      },
      { source: "/settings", destination: "/panel/settings", permanent: false },
    ];
  },
};

export default nextConfig;
