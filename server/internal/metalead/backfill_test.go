package metalead

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
)

// backfillGraph answers each list backfill reads, in Graph's own shapes.
func backfillGraph() fakeGraph {
	return fakeGraph{objects: map[string]string{
		"page1/conversations?platform=messenger": `{"data":[{"id":"t_1","messages":{"data":[
			{"id":"m3","message":"thanks, booking now","from":{"id":"page1","name":"Us"},"created_time":"2026-11-01T10:05:00+0000"},
			{"id":"m2","message":"how much for a website? ada@example.com","from":{"id":"psid1","name":"Ada"},"created_time":"2026-11-01T10:02:00+0000"},
			{"id":"m1","message":"hi there","from":{"id":"psid1","name":"Ada"},"created_time":"2026-11-01T10:00:00+0000"}
		]}}]}`,
		"page1/feed": `{"data":[
			{"id":"page1_9","message":"Our new range","from":{"id":"page1"},"permalink_url":"https://fb/p9","comments":{"data":[
				{"id":"page1_9_1","message":"price for 10?","from":{"id":"x1","name":"Cy"},"created_time":"2026-11-01T09:00:00+0000"},
				{"id":"page1_9_2","message":"DM us","from":{"id":"page1","name":"Us"}}
			]}},
			{"id":"page1_8","message":"Do you deliver to Leeds?","from":{"id":"x2","name":"Di"},"permalink_url":"https://fb/p8","created_time":"2026-10-30T09:00:00+0000"},
			{"id":"page1_7","message":"no author shown"}
		]}`,
		"page1/tagged": `{"data":[{"id":"z_7","message":"loving @us","from":{"id":"x3","name":"Ed"},"permalink_url":"https://fb/z7"}]}`,
		"page1/conversations?platform=instagram": `{"data":[{"id":"t_2","messages":{"data":[
			{"id":"im1","message":"do you ship to UK?","from":{"id":"igsid1","username":"bo"},"created_time":"2026-11-02T08:00:00+0000"}
		]}}]}`,
		"ig1/media": `{"data":[{"id":"md1","permalink":"https://ig/p/1","comments":{"data":[
			{"id":"c1","text":"need 3 of these","username":"flo","from":{"id":"u9","username":"flo"},"timestamp":"2026-11-02T07:00:00+0000"},
			{"id":"c2","text":"thanks!","from":{"id":"ig1","username":"us"}}
		]}}]}`,
		"ig1/tags": `{"data":[{"id":"md9","caption":"new shelf from @us","username":"gus","permalink":"https://ig/p/9","timestamp":"2026-10-01T00:00:00+0000"}]}`,
	}}
}

func TestBackfillInbox(t *testing.T) {
	svc := NewService(newMemStore(), backfillGraph(), nil, Config{}, nil)
	conn := &connRow{MetaLeadConnection: model.MetaLeadConnection{PageID: "page1", IGAccountID: "ig1"}}
	its, warns := svc.backfillInbox(context.Background(), conn, "tok", nil, 50)
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	want := "facebook_dm=dm:page1:psid1 facebook_comment=page1_9_1 facebook_comment=page1_8 facebook_mention=z_7 " +
		"instagram_dm=dm:ig1:igsid1 instagram_comment=c1 instagram_tag=md9"
	if got := keys(its); got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	dm, comment, tag := its[0], its[1], its[6]
	if dm.Text != "hi there\nhow much for a website? ada@example.com" || dm.EventID != "m2" || dm.AuthorName != "Ada" {
		t.Fatalf("thread = %+v", dm)
	}
	if comment.Permalink != "https://fb/p9" || comment.AuthorName != "Cy" || comment.At == nil {
		t.Fatalf("comment = %+v", comment)
	}
	if tag.Text != "new shelf from @us" || tag.AuthorName != "gus" || tag.Permalink != "https://ig/p/9" {
		t.Fatalf("tag = %+v", tag)
	}

	// A Page with no Instagram account and no messaging permission still
	// backfills the rest.
	g := backfillGraph()
	delete(g.objects, "page1/conversations?platform=messenger")
	svc = NewService(newMemStore(), g, nil, Config{}, nil)
	its, warns = svc.backfillInbox(context.Background(), &connRow{MetaLeadConnection: model.MetaLeadConnection{PageID: "page1"}}, "tok", nil, 50)
	if len(warns) != 1 || !strings.HasPrefix(warns[0], "Facebook DMs: ") || len(its) != 3 {
		t.Fatalf("warns = %v its = %s", warns, keys(its))
	}
}

// Each list is read past its first page: by cursor or by time, as Graph's next
// link says, until the list has given enough items or reaches since.
func TestBackfillInboxPages(t *testing.T) {
	// next links are Graph's own: another host, our token in them. Only their
	// paging parameters may be reused.
	next := func(edge, param string) string {
		return `"paging":{"next":"https://graph.facebook.com/v26.0/page1/` + edge + `?limit=25&` + param + `&access_token=leaked"}`
	}
	g := fakeGraph{objects: map[string]string{
		"page1/feed": `{"data":[{"id":"p3","message":"price list?","from":{"id":"x1"},"created_time":"2026-11-03T09:00:00+0000"}],` +
			next("feed", "after=c2") + `}`,
		"page1/feed#c2": `{"data":[{"id":"p2","message":"open on Sunday?","from":{"id":"x2"},"created_time":"2026-10-20T09:00:00+0000"}],` +
			next("feed", "after=c1") + `}`,
		"page1/feed#c1": `{"data":[{"id":"p1","message":"old question","from":{"id":"x3"},"created_time":"2026-09-01T09:00:00+0000"}]}`,
		"page1/tagged": `{"data":[{"id":"t2","message":"@us rocks","from":{"id":"x4"},"created_time":"2026-11-02T09:00:00+0000"}],` +
			next("tagged", "until=1761900000&__paging_token=tok2") + `}`,
		"page1/tagged#tok2": `{"data":[{"id":"t1","message":"@us again","from":{"id":"x5"},"created_time":"2026-10-25T09:00:00+0000"}],"paging":{}}`,
	}}
	svc := NewService(newMemStore(), g, nil, Config{}, nil)
	conn := &connRow{MetaLeadConnection: model.MetaLeadConnection{PageID: "page1"}}
	read := func(since *time.Time, want int) (string, []string) {
		its, warns := svc.backfillInbox(context.Background(), conn, "tok", since, want)
		return keys(its), warns
	}

	// Every page, cursor and time paging alike (the DM list is refused).
	got, warns := read(nil, 50)
	if got != "facebook_comment=p3 facebook_comment=p2 facebook_comment=p1 facebook_mention=t2 facebook_mention=t1" || len(warns) != 1 {
		t.Fatalf("got %s warns %v", got, warns)
	}
	// since 2026-10-22: the feed's second page already reaches it, so its
	// third is never read; the run drops p2 itself.
	cut := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	delete(g.objects, "page1/feed#c1")
	if got, warns = read(&cut, 50); got != "facebook_comment=p3 facebook_comment=p2 facebook_mention=t2 facebook_mention=t1" || len(warns) != 1 {
		t.Fatalf("since: got %s warns %v", got, warns)
	}
	// One wanted: one page per list.
	delete(g.objects, "page1/feed#c2")
	delete(g.objects, "page1/tagged#tok2")
	if got, warns = read(nil, 1); got != "facebook_comment=p3 facebook_mention=t2" || len(warns) != 1 {
		t.Fatalf("want 1: got %s warns %v", got, warns)
	}
	// A page Graph refuses keeps what came before it and says how far it got.
	if got, warns = read(nil, 50); !strings.Contains(strings.Join(warns, " | "), "Facebook comments, page 2: ") ||
		got != "facebook_comment=p3 facebook_mention=t2" {
		t.Fatalf("refused page: got %s warns %v", got, warns)
	}
}

// A launch keeps the newest items, folds a backfilled thread over the
// messages a webhook already stored instead of repeating them, and skips what
// it already has.
func TestLaunchBackfillMergesWithWebhook(t *testing.T) {
	tokenKey := "test-meta-token-key-32bytes!!!!"
	key, _ := mail.KeyFromSecret(tokenKey)
	enc, _ := mail.Encrypt(key, []byte("page-token"))
	store := newMemStore()
	org := uuid.New()
	conn := &connRow{
		MetaLeadConnection: model.MetaLeadConnection{ID: uuid.New(), OrgID: org, Name: "shop", PageID: "page1", IGAccountID: "ig1"},
		AccessTokenEnc:     enc,
	}
	store.conns["shop"], store.byPage["page1"], store.byPage["ig1"] = conn, conn, conn
	// From webhooks: the thread's first message, and comment c1.
	store.leads["dm:page1:psid1"] = &model.MetaLead{ID: uuid.New(), OrgID: org, LeadgenID: "dm:page1:psid1", Source: model.MetaLeadSourceFBDM, Body: "hi there"}
	store.lastEvent["dm:page1:psid1"] = "m1"
	store.leads["c1"] = &model.MetaLead{ID: uuid.New(), OrgID: org, LeadgenID: "c1", Source: model.MetaLeadSourceIGComment, Body: "need 3 of these"}
	store.lastEvent["c1"] = "c1"

	var calls int
	llm := funcLLM(func(prompt string) (string, error) {
		calls++
		if strings.Contains(prompt, "website") {
			return `{"is_lead":true,"category":"pricing","fit_score":70,"evidence":"how much for a website?","summary":"Website price"}`, nil
		}
		return `{"is_lead":false,"category":"other","fit_score":10,"summary":"Other"}`, nil
	})
	crm := &stubCRM{}
	svc := NewService(store, backfillGraph(), &Analyser{LLM: llm}, Config{TokenKey: tokenKey, MaxLeadsPerLaunch: 50}, nil)
	svc.BindCRM(crm)

	res, err := svc.Launch(context.Background(), LaunchRequest{
		OrgID: org, UserID: uuid.New(), Connection: "shop", Include: IncludeInbox, Limit: 4, PushCRM: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var run *model.MetaLeadRun
	waitFor(t, func() bool {
		run, _ = store.GetRun(context.Background(), org, res.RunID)
		return run != nil && run.Status == model.MetaLeadRunStatusDone
	})
	// Newest four: the IG DM, c1 (already stored), the FB thread, comment page1_9_1.
	if run.LeadCount != 3 || run.ErrorMessage != "" {
		t.Fatalf("run = %+v", run)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	thread := store.leads["dm:page1:psid1"]
	if thread.Body != "hi there\nhow much for a website? ada@example.com" || thread.RunID == nil || *thread.RunID != res.RunID {
		t.Fatalf("thread = %+v", thread)
	}
	if store.leads["c1"].Analysis != nil || store.leads["page1_8"] != nil {
		t.Fatalf("c1 re-analysed or page1_8 kept past the limit")
	}
	if calls != 4 || len(crm.calls) != 1 || crm.calls[0].Email != "ada@example.com" { // 3 briefs + the rollup
		t.Fatalf("llm calls = %d crm = %+v", calls, crm.calls)
	}
}

type funcLLM func(prompt string) (string, error)

func (f funcLLM) Complete(_ context.Context, _, prompt string, _ int, _ bool) (string, error) {
	return f(prompt)
}
