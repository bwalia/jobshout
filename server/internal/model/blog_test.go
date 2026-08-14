package model

import "testing"

func TestGenerateBlogRequest_Normalize(t *testing.T) {
	tests := []struct {
		name       string
		req        GenerateBlogRequest
		wantTopics []string
		wantCtx    []string
	}{
		{
			name:       "briefs pass through",
			req:        GenerateBlogRequest{Briefs: []BlogBrief{{Topic: "Gateway API", Context: "for platform engineers"}}},
			wantTopics: []string{"Gateway API"},
			wantCtx:    []string{"for platform engineers"},
		},
		{
			// The legacy shape must keep working: stored scheduled tasks and
			// the retry path both still send it.
			name:       "legacy topics become briefs",
			req:        GenerateBlogRequest{Topics: []string{"Postgres tuning", "Kafka rebalancing"}},
			wantTopics: []string{"Postgres tuning", "Kafka rebalancing"},
			wantCtx:    []string{"", ""},
		},
		{
			name: "both are kept, briefs first",
			req: GenerateBlogRequest{
				Briefs: []BlogBrief{{Topic: "A", Context: "ctx"}},
				Topics: []string{"B"},
			},
			wantTopics: []string{"A", "B"},
			wantCtx:    []string{"ctx", ""},
		},
		{
			name: "blank topics are dropped",
			req: GenerateBlogRequest{
				Briefs: []BlogBrief{{Topic: "  "}, {Topic: "Real"}},
				Topics: []string{"", "   "},
			},
			wantTopics: []string{"Real"},
			wantCtx:    []string{""},
		},
		{
			name:       "whitespace is trimmed",
			req:        GenerateBlogRequest{Briefs: []BlogBrief{{Topic: "  Spaced  ", Context: "  ctx  "}}},
			wantTopics: []string{"Spaced"},
			wantCtx:    []string{"ctx"},
		},
		{
			name:       "empty stays empty",
			req:        GenerateBlogRequest{},
			wantTopics: []string{},
			wantCtx:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Normalize()

			if len(tt.req.Briefs) != len(tt.wantTopics) {
				t.Fatalf("got %d briefs, want %d: %+v", len(tt.req.Briefs), len(tt.wantTopics), tt.req.Briefs)
			}
			for i, want := range tt.wantTopics {
				if tt.req.Briefs[i].Topic != want {
					t.Errorf("brief %d topic = %q, want %q", i, tt.req.Briefs[i].Topic, want)
				}
				if tt.req.Briefs[i].Context != tt.wantCtx[i] {
					t.Errorf("brief %d context = %q, want %q", i, tt.req.Briefs[i].Context, tt.wantCtx[i])
				}
			}

			// Topics must stay in step with Briefs — the column, the retry path
			// and the legacy readers all depend on that.
			if len(tt.req.Topics) != len(tt.wantTopics) {
				t.Fatalf("got %d topics, want %d: %v", len(tt.req.Topics), len(tt.wantTopics), tt.req.Topics)
			}
			for i, want := range tt.wantTopics {
				if tt.req.Topics[i] != want {
					t.Errorf("topic %d = %q, want %q", i, tt.req.Topics[i], want)
				}
			}
		})
	}
}

func TestGenerateBlogRequest_NormalizeIsIdempotent(t *testing.T) {
	// Normalize appends Topics into Briefs, so running it twice must not
	// duplicate every entry.
	req := GenerateBlogRequest{Briefs: []BlogBrief{{Topic: "A", Context: "ctx"}}}

	req.Normalize()
	first := len(req.Briefs)
	req.Normalize()

	if len(req.Briefs) != first {
		t.Errorf("got %d briefs after a second Normalize, want %d", len(req.Briefs), first)
	}
	if req.Briefs[0].Context != "ctx" {
		t.Errorf("context was lost on the second pass: %q", req.Briefs[0].Context)
	}
}

func TestGenerateBlogRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     GenerateBlogRequest
		wantErr bool
	}{
		{
			name:    "briefs present",
			req:     GenerateBlogRequest{Briefs: []BlogBrief{{Topic: "A"}}},
			wantErr: false,
		},
		{
			name:    "nothing to write",
			req:     GenerateBlogRequest{},
			wantErr: true,
		},
		{
			// Reserved in the contract but not yet honoured. Refusing is the
			// honest answer — accepting it would silently write nothing.
			name:    "trending is refused until discovery lands",
			req:     GenerateBlogRequest{Trending: true, TrendingCount: 3},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr && err == nil {
				t.Error("Validate() = nil, want an error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}
