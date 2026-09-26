import { JobCardSkeleton } from "@/components/JobCard";

export default function JobsLoading() {
  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <div className="skeleton h-12 w-64 rounded-lg" />
      <div className="skeleton mt-4 h-5 w-96 max-w-full rounded" />
      <div className="skeleton mt-8 h-[4.5rem] w-full rounded-card" />
      <div className="mt-8 grid gap-8 lg:grid-cols-[16rem_1fr] lg:gap-12">
        <div className="hidden space-y-4 lg:block">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="skeleton h-28 rounded-card" />
          ))}
        </div>
        <ul className="grid gap-4 xl:grid-cols-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <li key={i}>
              <JobCardSkeleton />
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
