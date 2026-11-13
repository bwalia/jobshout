import { redirect } from "next/navigation";

/** Freelancers fold into the members talent directory (Phase 2a). */
export default function FreelancersRedirectPage() {
  redirect("/members?type=freelancer");
}
