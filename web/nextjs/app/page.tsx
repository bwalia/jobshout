import { redirect } from "next/navigation";

export default function HomePage() {
  // Root page redirects to dashboard (or login if not authenticated)
  redirect("/dashboard");
}
