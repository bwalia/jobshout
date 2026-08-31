import { Zap } from "lucide-react";
import { LoginBrand } from "@/components/layout/LoginBrand";

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-screen bg-background">
      <LoginBrand />

      <div className="flex flex-1 flex-col items-center justify-center px-6 py-12">
        <div className="w-full max-w-md">
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
