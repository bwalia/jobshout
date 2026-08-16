package scheduler

import (
	"testing"

	"github.com/jobshout/server/internal/model"
)

func TestIsBlogTask(t *testing.T) {
	tests := []struct {
		name string
		task model.ScheduledTask
		want bool
	}{
		{
			name: "task type",
			task: model.ScheduledTask{TaskType: "blog"},
			want: true,
		},
		{
			// Predates the task type. Tasks created this way are sitting in the
			// database and must keep firing.
			name: "legacy tag",
			task: model.ScheduledTask{TaskType: "agent", Tags: []string{"blog"}},
			want: true,
		},
		{
			name: "legacy input marker",
			task: model.ScheduledTask{TaskType: "agent", InputJSON: map[string]any{"kind": "blog"}},
			want: true,
		},
		{
			name: "unrelated task",
			task: model.ScheduledTask{TaskType: "agent", Tags: []string{"reports"}},
			want: false,
		},
		{
			name: "unrelated tag containing blog is not a match",
			task: model.ScheduledTask{TaskType: "workflow", Tags: []string{"blogging-team"}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBlogTask(tt.task); got != tt.want {
				t.Errorf("isBlogTask() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlogRequestFromInput(t *testing.T) {
	tests := []struct {
		name       string
		input      map[string]any
		wantErr    bool
		wantTopics []string
		wantTrend  bool
	}{
		{
			name:       "briefs",
			input:      map[string]any{"briefs": []any{map[string]any{"topic": "Gateway API", "context": "for operators"}}},
			wantTopics: []string{"Gateway API"},
		},
		{
			// Tasks stored before briefs existed carry a bare topics array.
			name:       "legacy topics",
			input:      map[string]any{"topics": []any{"Postgres tuning"}},
			wantTopics: []string{"Postgres tuning"},
		},
		{
			// The point of the whole feature: a recurring task with no subject,
			// which discovers one each time it fires.
			name:      "trending needs no topics",
			input:     map[string]any{"trending": true, "trending_count": float64(2)},
			wantTrend: true,
		},
		{
			name:    "neither topics nor trending is an error",
			input:   map[string]any{"model": "llama3"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := blogRequestFromInput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("blogRequestFromInput accepted an input with nothing to write")
				}
				return
			}
			if err != nil {
				t.Fatalf("blogRequestFromInput: %v", err)
			}
			if got.Trending != tt.wantTrend {
				t.Errorf("Trending = %v, want %v", got.Trending, tt.wantTrend)
			}
			if len(got.Briefs) != len(tt.wantTopics) {
				t.Fatalf("got %d briefs, want %d", len(got.Briefs), len(tt.wantTopics))
			}
			for i, want := range tt.wantTopics {
				if got.Briefs[i].Topic != want {
					t.Errorf("brief %d = %q, want %q", i, got.Briefs[i].Topic, want)
				}
			}
		})
	}
}

// A trending task must survive the round trip through input_json, which is how
// it is stored and how the dispatcher reads it back every time it fires.
func TestBlogRequestFromInput_TrendingCountSurvives(t *testing.T) {
	got, err := blogRequestFromInput(map[string]any{
		"trending":       true,
		"trending_count": float64(3),
		"model":          "llama3",
	})
	if err != nil {
		t.Fatalf("blogRequestFromInput: %v", err)
	}
	if got.TrendingCount != 3 {
		t.Errorf("TrendingCount = %d, want 3", got.TrendingCount)
	}
	if got.Model != "llama3" {
		t.Errorf("Model = %q, want llama3", got.Model)
	}
}
