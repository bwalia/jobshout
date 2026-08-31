"use client";

interface NodePaletteProps {
  onAddNode: (type: string) => void;
}

const nodeTypes = [
  {
    type: "llm",
    label: "LLM",
    icon: "LLM",
    color: "bg-indigo-100 text-indigo-700 border-indigo-300 dark:bg-indigo-500/20 dark:text-indigo-400 dark:border-indigo-500/50",
    description: "Language model inference",
  },
  {
    type: "tool",
    label: "Tool",
    icon: "T",
    color: "bg-amber-100 text-amber-800 border-amber-300 dark:bg-amber-500/20 dark:text-amber-400 dark:border-amber-500/50",
    description: "External tool invocation",
  },
  {
    type: "decision",
    label: "Decision",
    icon: "?",
    color: "bg-pink-100 text-pink-800 border-pink-300 dark:bg-pink-500/20 dark:text-pink-400 dark:border-pink-500/50",
    description: "Conditional branching",
  },
  {
    type: "agent",
    label: "Agent",
    icon: "A",
    color: "bg-cyan-100 text-cyan-800 border-cyan-300 dark:bg-cyan-500/20 dark:text-cyan-400 dark:border-cyan-500/50",
    description: "Autonomous agent step",
  },
];

export function NodePalette({ onAddNode }: NodePaletteProps) {
  return (
    <div className="space-y-2">
      {nodeTypes.map((nodeType) => (
        <button
          key={nodeType.type}
          onClick={() => onAddNode(nodeType.type)}
          className={`w-full flex items-center gap-3 rounded-md border px-3 py-2 text-left transition-colors hover:bg-accent ${nodeType.color}`}
        >
          <div className="h-7 w-7 rounded flex items-center justify-center text-xs font-bold shrink-0">
            {nodeType.icon}
          </div>
          <div className="min-w-0">
            <p className="text-sm font-medium text-foreground">{nodeType.label}</p>
            <p className="text-xs text-muted-foreground truncate">{nodeType.description}</p>
          </div>
        </button>
      ))}
    </div>
  );
}
