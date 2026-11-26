package facebook

import (
	"strings"
	"testing"
	"time"
)

var calNow = time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC) // a Monday

func calReq() DraftRequest {
	return DraftRequest{Page: PageRecord{Name: "Acme", Timezone: "Europe/London"}, Days: 7, Count: 2, Now: calNow}
}

func TestParseCalendarComposesAndLimitsHashtags(t *testing.T) {
	reply := "```json\n" + `[
	 {"category":"Educational","audience":"homeowners","objective":"engagement",
	  "opening":"Is your boiler ready?","body":"Three checks before winter.","cta":"Book a service today.",
	  "hashtags":["#Boiler","#Winter","#Home","#Extra"],"image_prompt":"a warm kitchen","day_offset":2,"time":"18:30"},
	 {"category":"made-up","opening":"Hello","body":"","cta":"","hashtags":"#One #Two","day_offset":"x","time":"late"}
	]` + "\n```"
	got, err := ParseCalendar(reply, calReq())
	if err != nil || len(got) != 2 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if got[0].Category != "educational" || got[1].Category != "informational" {
		t.Errorf("categories = %q, %q", got[0].Category, got[1].Category)
	}
	want := "Is your boiler ready?\n\nThree checks before winter.\n\nBook a service today.\n\n#Boiler #Winter #Home"
	if got[0].Message != want {
		t.Errorf("message = %q", got[0].Message)
	}
	if got[1].Message != "Hello\n\n#One #Two" {
		t.Errorf("message 2 = %q", got[1].Message)
	}
	// 18:30 London on 4 March (GMT) is 18:30 UTC.
	if at := *got[0].SuggestedFor; !at.Equal(time.Date(2026, 3, 4, 18, 30, 0, 0, time.UTC)) {
		t.Errorf("suggested = %v", at)
	}
	if !got[1].SuggestedFor.After(calNow) {
		t.Errorf("fallback time %v not in the future", got[1].SuggestedFor)
	}
}

func TestParseCalendarToleratesShapes(t *testing.T) {
	for name, reply := range map[string]string{
		"wrapped":  `{"posts":[{"opening":"A","body":"B"}]}`,
		"thinking": `<think>hmm [x]</think>[{"opening":"A","cta":["Call","us"],"day_offset":3}]`,
		"numbers":  `[{"opening":"A","hashtags":5,"day_offset":3,"time":930}]`,
	} {
		got, err := ParseCalendar(reply, calReq())
		if err != nil || len(got) != 1 {
			t.Errorf("%s: got=%v err=%v", name, got, err)
		}
	}
}

func TestParseCalendarRejectsGarbage(t *testing.T) {
	for _, reply := range []string{"", "sorry, I can't", `[]`, `[{"category":"x"}]`} {
		if _, err := ParseCalendar(reply, calReq()); err == nil {
			t.Errorf("accepted %q", reply)
		}
	}
}

func TestSuggestTimeNeverPast(t *testing.T) {
	loc := time.UTC
	// Day 1 at 09:05 is only 5 minutes away; it must roll to tomorrow.
	got := suggestTime(calNow, loc, "1", "09:05", 0, 1, 7)
	if !got.After(calNow.Add(15 * time.Minute)) {
		t.Errorf("got %v", got)
	}
}

func TestFitMessageCutsAtParagraph(t *testing.T) {
	para := strings.Repeat("a", 3000)
	got := FitMessage(para + "\n\n" + para)
	if got != para {
		t.Errorf("len = %d", len(got))
	}
}

func TestImagePromptForbidsLettering(t *testing.T) {
	p := ImagePrompt("a bakery window", PageRecord{Voice: "warm and cheeky"})
	for _, w := range []string{"No text", "logos", "warm and cheeky"} {
		if !strings.Contains(p, w) {
			t.Errorf("prompt missing %q: %s", w, p)
		}
	}
}

func TestCalendarPromptDoesNotInviteInventedFacts(t *testing.T) {
	r := calReq()
	if p := calendarPrompt(r); !strings.Contains(p, "do not mention any") {
		t.Error("no-facts rule missing when no notes were given")
	}
	r.Notes = "10% off until Friday"
	p := calendarPrompt(r)
	if !strings.Contains(p, "10% off until Friday") || strings.Contains(p, "do not mention any") {
		t.Errorf("notes not carried: %s", p)
	}
}

func TestSummariseAndHint(t *testing.T) {
	posts := []Post{
		{Category: "educational", Reactions: 10, Comments: 2, Shares: 1},
		{Category: "educational", Reactions: 8, Comments: 2},
		{Category: "offer", Reactions: 1},
		{Category: "offer", Reactions: 2},
		{Category: "brand", Reactions: 3},
	}
	a := Summarise(PageRecord{PageID: "1", Followers: 50}, posts)
	if a.Posts != 5 || a.Reactions != 24 || a.Top == nil || a.Top.Reactions != 10 {
		t.Errorf("summary = %+v", a)
	}
	if a.ByCategory[0].Category != "educational" || a.ByCategory[0].AvgPerPost != 11.5 {
		t.Errorf("by category = %+v", a.ByCategory)
	}
	h := performanceHint(a)
	if !strings.Contains(h, "educational") || !strings.Contains(h, "offer") {
		t.Errorf("hint = %q", h)
	}
	if performanceHint(Summarise(PageRecord{}, posts[:2])) != "" {
		t.Error("hint given on too little data")
	}
}
