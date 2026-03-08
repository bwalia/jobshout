"use client";

interface CategoryFilterProps {
  categories: string[];
  activeCategory: string;
  onCategoryChange: (category: string) => void;
}

export function CategoryFilter({
  categories,
  activeCategory,
  onCategoryChange,
}: CategoryFilterProps) {
  return (
    // overflow-x-auto with no-scrollbar allows horizontal scrolling on small screens
    <div className="relative">
      <div className="flex gap-2 overflow-x-auto pb-1 scrollbar-none">
        {categories.map((category) => {
          const isActive = category === activeCategory;
          return (
            <button
              key={category}
              type="button"
              onClick={() => onCategoryChange(category)}
              className={[
                "inline-flex shrink-0 items-center rounded-full px-4 py-1.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                isActive
                  ? "bg-primary text-primary-foreground"
                  : "border border-border bg-background text-foreground hover:bg-accent hover:text-accent-foreground",
              ].join(" ")}
            >
              {category}
            </button>
          );
        })}
      </div>
    </div>
  );
}
