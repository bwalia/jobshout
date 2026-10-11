package blog

import (
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
)

func TestIndustryBlockCarriesWriterStyle(t *testing.T) {
	if got := industryBlock(model.BlogBrief{}); got != "" {
		t.Errorf("no industry, no style = %q, want empty", got)
	}
	got := industryBlock(model.BlogBrief{Industry: "FinTech", Style: "Short tips, plain words."})
	for _, want := range []string{"FinTech", "HOUSE STYLE", "Short tips, plain words.", "never override the rules on sources"} {
		if !strings.Contains(got, want) {
			t.Errorf("block missing %q:\n%s", want, got)
		}
	}
}
