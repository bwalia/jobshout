package metalead

// Backfill reads what the webhooks would have delivered before a Page was
// connected or subscribed: recent DM threads, comments and visitor posts, posts
// the Page is tagged in, and Instagram posts the account is photo-tagged in,
// which Meta sends no webhook for at all. Items get the same keys the webhooks
// use, so one already stored is not analysed twice.

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jobshout/server/internal/model"
)

// Launch include modes.
const (
	IncludeAll   = "all"
	IncludeForms = "forms"
	IncludeInbox = "inbox"
)

// backfillPage is the page size each Graph list is read in, newest first, and
// how many comments are read per post.
// ponytail: a post's comments are its newest 25 (nested lists are not paged);
// page them too if busy posts lose leads.
const backfillPage = "25"

type graphFrom struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

// graphItem is the union of the Graph objects backfill reads: conversations,
// messages, posts, comments and media.
type graphItem struct {
	ID           string     `json:"id"`
	Message      string     `json:"message"` // Facebook post, comment, DM
	Text         string     `json:"text"`    // Instagram comment
	Caption      string     `json:"caption"` // Instagram media
	Username     string     `json:"username"`
	From         graphFrom  `json:"from"`
	PermalinkURL string     `json:"permalink_url"` // Facebook
	Permalink    string     `json:"permalink"`     // Instagram
	CreatedTime  string     `json:"created_time"`  // Facebook
	Timestamp    string     `json:"timestamp"`     // Instagram
	UpdatedTime  string     `json:"updated_time"`  // conversation
	Comments     *graphList `json:"comments"`
	Messages     *graphList `json:"messages"`
}

type graphList struct {
	Data   []graphItem `json:"data"`
	Paging graphPaging `json:"paging"`
}

func (g graphItem) when() string { return firstNonEmpty(g.CreatedTime, g.Timestamp, g.UpdatedTime) }

func (g graphItem) children(l *graphList) []graphItem {
	if l == nil {
		return nil
	}
	return l.Data
}

// asInteraction is one comment, post or media as an interaction; link is the
// post's, for a comment that has none of its own.
func asInteraction(src, account string, g graphItem, link string) interaction {
	return interaction{
		AccountID: account, Source: src, Key: g.ID, EventID: g.ID,
		AuthorID: g.From.ID, AuthorName: firstNonEmpty(g.From.Username, g.Username, g.From.Name),
		Text: firstNonEmpty(g.Message, g.Text, g.Caption), Permalink: firstNonEmpty(g.PermalinkURL, g.Permalink, link),
		At: ParseCreatedTime(g.when()),
	}
}

// olderThan reports whether a list item predates since, so the pages after it
// (Graph lists newest first) need not be read. An item with no time is not.
func olderThan(g graphItem, since *time.Time) bool {
	at := ParseCreatedTime(g.when())
	return since != nil && at != nil && at.Before(*since)
}

// dmThread folds a conversation's messages from the customer, oldest first,
// into one interaction keyed like the webhook's thread. Graph shows only a
// conversation's 20 newest messages.
func dmThread(c graphItem, src, account string, own func(string) bool) (interaction, bool) {
	var msgs []graphItem
	for _, m := range c.children(c.Messages) {
		if !own(m.From.ID) && strings.TrimSpace(m.Message) != "" {
			msgs = append(msgs, m)
		}
	}
	if len(msgs) == 0 {
		return interaction{}, false
	}
	// Graph's timestamps are all UTC "2006-01-02T15:04:05+0000", so they sort as text.
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].when() < msgs[j].when() })
	texts := make([]string, len(msgs))
	for i, m := range msgs {
		texts[i] = m.Message
	}
	for len(texts) > 1 && len(strings.Join(texts, "\n")) > 4000 {
		texts = texts[1:] // the body cap: keep the newest messages whole
	}
	newest := msgs[len(msgs)-1]
	it := asInteraction(src, account, newest, "")
	it.Key = "dm:" + account + ":" + newest.From.ID
	it.Text = strings.Join(texts, "\n")
	return it, true
}

// backfillInbox lists a connection's recent DMs, comments and tags. Each list
// is paged, newest first, until it has given want items or reaches since. A
// list Graph refuses (a permission not granted, say) is a warning, not a
// failure: the rest still count.
func (s *Service) backfillInbox(ctx context.Context, conn *connRow, token string, since *time.Time, want int) ([]interaction, []string) {
	var out []interaction
	var warns []string
	read := func(label, path string, q url.Values, each func(graphItem)) {
		q.Set("limit", backfillPage)
		start := len(out)
		for page := 0; page < maxGraphPages; page++ {
			var l graphList
			if err := s.graph.Get(ctx, path, q, token, &l); err != nil {
				if page > 0 {
					label = fmt.Sprintf("%s, page %d", label, page+1)
				}
				warns = append(warns, label+": "+err.Error())
				return
			}
			for _, g := range l.Data {
				each(g)
			}
			if q = nextPage(l.Paging); q == nil || len(out)-start >= want || len(l.Data) == 0 ||
				olderThan(l.Data[len(l.Data)-1], since) {
				return
			}
		}
	}
	page, ig := conn.PageID, conn.IGAccountID
	own := func(id string) bool { return id != "" && (id == page || id == ig) }
	add := func(it interaction) {
		if strings.TrimSpace(it.Text) != "" && it.Key != "" {
			out = append(out, it)
		}
	}
	threads := func(src, account string) func(graphItem) {
		return func(c graphItem) {
			if it, ok := dmThread(c, src, account, own); ok {
				add(it)
			}
		}
	}
	const convFields = "updated_time,messages.limit(20){id,message,from,created_time}"

	read("Facebook DMs", page+"/conversations", url.Values{"platform": {"messenger"}, "fields": {convFields}},
		threads(model.MetaLeadSourceFBDM, page))
	read("Facebook comments", page+"/feed",
		fields("id,message,from,permalink_url,created_time,comments.limit("+backfillPage+"){id,message,from,permalink_url,created_time}"),
		func(p graphItem) {
			// A visitor's post. One whose author Graph will not name is taken
			// to be the Page's own.
			if p.From.ID != "" && !own(p.From.ID) {
				add(asInteraction(model.MetaLeadSourceFBComment, page, p, ""))
			}
			for _, c := range p.children(p.Comments) {
				if !own(c.From.ID) {
					add(asInteraction(model.MetaLeadSourceFBComment, page, c, p.PermalinkURL))
				}
			}
		})
	read("Facebook tags", page+"/tagged", fields("id,message,from,permalink_url,created_time"), func(p graphItem) {
		add(asInteraction(model.MetaLeadSourceFBMention, page, p, ""))
	})
	if ig == "" {
		return out, warns
	}
	read("Instagram DMs", page+"/conversations", url.Values{"platform": {"instagram"}, "fields": {convFields}},
		threads(model.MetaLeadSourceIGDM, ig))
	read("Instagram comments", ig+"/media",
		fields("id,permalink,comments.limit("+backfillPage+"){id,text,username,from,timestamp}"),
		func(m graphItem) {
			for _, c := range m.children(m.Comments) {
				if !own(c.From.ID) {
					add(asInteraction(model.MetaLeadSourceIGComment, ig, c, m.Permalink))
				}
			}
		})
	read("Instagram tags", ig+"/tags", fields("id,caption,username,permalink,timestamp"), func(m graphItem) {
		add(asInteraction(model.MetaLeadSourceIGTag, ig, m, ""))
	})
	return out, warns
}
