package llmtrace

import (
	"context"
	"testing"
)

func TestWithTraceInheritsTaskRunLink(t *testing.T) {
	ctx := WithTaskRun(context.Background(), "task-1", "taskrun-1")
	ctx = WithTrace(ctx, TraceInfo{TraceName: "go-executor-run", SessionID: "exec-1", ExecutionID: "exec-1"})
	info, _ := FromContext(ctx)
	if info.TaskID != "task-1" || info.TaskRunID != "taskrun-1" || info.ExecutionID != "exec-1" || info.RunKind != "go-executor-run" {
		t.Errorf("info = %+v", info)
	}
}

func TestWithTraceOwnTaskWins(t *testing.T) {
	ctx := WithTaskRun(context.Background(), "outer-task", "outer-run")
	ctx = WithTrace(ctx, TraceInfo{TraceName: "go-course-run", TaskID: "own-task"})
	info, _ := FromContext(ctx)
	if info.TaskID != "own-task" || info.TaskRunID != "outer-run" {
		t.Errorf("info = %+v", info)
	}
}

func TestWithTraceWithoutOuterLinkLeavesTaskEmpty(t *testing.T) {
	info, _ := FromContext(WithTrace(context.Background(), TraceInfo{TraceName: "go-chat-run"}))
	if info.TaskID != "" || info.TaskRunID != "" {
		t.Errorf("info = %+v", info)
	}
}
