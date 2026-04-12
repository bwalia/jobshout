import { Zap } from "lucide-react";

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-screen bg-background">
      {/* Left panel — brand */}
      <div className="hidden lg:flex lg:w-1/2 lg:flex-col lg:justify-between bg-sidebar p-10">
        <div className="flex items-center gap-2.5">
          <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Zap className="h-5 w-5" />
          </span>
          <span className="text-lg font-semibold tracking-tight text-white">
            Jobshout
          </span>
        </div>
        <div className="max-w-md">
          <h2 className="text-3xl font-bold tracking-tight text-white">
            AI Team Command Center
          </h2>
          <p className="mt-3 text-base text-slate-400">
            Mission control for AI teams. Create agents, build teams, assign
            projects, track work, and automate workflows.
          </p>
        </div>
        <p className="text-xs text-slate-500">
          Jobshout v0.3.0
        </p>
      </div>

      {/* Right panel — form */}
      <div className="flex flex-1 flex-col items-center justify-center px-6 py-12">
        <div className="w-full max-w-md">
          {/* Mobile logo */}
          <div className="mb-8 flex items-center justify-center gap-2.5 lg:hidden">
            <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary text-primary-foreground">
              <Zap className="h-5 w-5" />
            </span>
            <span className="text-lg font-semibold tracking-tight text-foreground">
              Jobshout
            </span>
          </div>
          {children}
        </div>
      </div>
    </div>
  );
}
