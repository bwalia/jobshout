import { Metadata } from "next";

export const metadata: Metadata = {
  title: "Projects",
  description: "Browse freelance projects and submit bids on JobShout",
};

export default function ProjectsLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <>{children}</>;
}