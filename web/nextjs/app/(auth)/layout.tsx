import { BetaBanner } from "@/components/layout/BetaBanner";
import { LoginBrand } from "@/components/layout/LoginBrand";
import { BrandLockup } from "@/components/brand/BrandMark";

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-screen flex-col bg-background">
      <BetaBanner />
      <div className="flex flex-1">
        <LoginBrand />

        <div className="flex flex-1 flex-col items-center justify-center px-6 py-12">
          <div className="w-full max-w-md">
            <div className="mb-8 flex justify-center lg:hidden">
              <BrandLockup />
            </div>
            {children}
          </div>
        </div>
      </div>
    </div>
  );
}
