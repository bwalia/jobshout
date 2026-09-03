package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/config"
	"github.com/jobshout/server/internal/googleauth"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type fakeGoogle struct {
	url     string
	profile googleauth.Profile
	err     error
}

func (f *fakeGoogle) AuthURL(state string) string { return f.url + "?state=" + state }
func (f *fakeGoogle) ProfileFromCode(context.Context, string) (googleauth.Profile, error) {
	return f.profile, f.err
}

type memUsers struct {
	byID    map[uuid.UUID]*model.User
	byEmail map[string]*model.User
	bySub   map[string]*model.User
	states  map[string]*model.GoogleOAuthState
	tickets map[string]*model.GoogleOAuthTicket
}

func newMemUsers() *memUsers {
	return &memUsers{
		byID:    map[uuid.UUID]*model.User{},
		byEmail: map[string]*model.User{},
		bySub:   map[string]*model.User{},
		states:  map[string]*model.GoogleOAuthState{},
		tickets: map[string]*model.GoogleOAuthTicket{},
	}
}

func cloneUser(u *model.User) *model.User {
	cp := *u
	if u.OrgID != nil {
		id := *u.OrgID
		cp.OrgID = &id
	}
	if u.GoogleSub != nil {
		s := *u.GoogleSub
		cp.GoogleSub = &s
	}
	if u.AvatarURL != nil {
		s := *u.AvatarURL
		cp.AvatarURL = &s
	}
	return &cp
}

func (m *memUsers) Create(_ context.Context, user *model.User) error {
	user.CreatedAt = time.Now()
	user.UpdatedAt = user.CreatedAt
	m.byID[user.ID] = cloneUser(user)
	m.byEmail[strings.ToLower(user.Email)] = m.byID[user.ID]
	if user.GoogleSub != nil {
		m.bySub[*user.GoogleSub] = m.byID[user.ID]
	}
	return nil
}
func (m *memUsers) FindByEmail(_ context.Context, email string) (*model.User, error) {
	u := m.byEmail[strings.ToLower(email)]
	if u == nil {
		return nil, nil
	}
	return cloneUser(u), nil
}
func (m *memUsers) FindByEmailFold(ctx context.Context, email string) (*model.User, error) {
	return m.FindByEmail(ctx, email)
}
func (m *memUsers) FindByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	u := m.byID[id]
	if u == nil {
		return nil, nil
	}
	return cloneUser(u), nil
}
func (m *memUsers) FindByGoogleSub(_ context.Context, sub string) (*model.User, error) {
	u := m.bySub[sub]
	if u == nil {
		return nil, nil
	}
	return cloneUser(u), nil
}
func (m *memUsers) UpdateOrgID(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *memUsers) UpdateProfile(_ context.Context, user *model.User) error {
	stored := m.byID[user.ID]
	if stored == nil {
		return nil
	}
	stored.FullName = user.FullName
	stored.AvatarURL = user.AvatarURL
	return nil
}
func (m *memUsers) LinkGoogleSub(_ context.Context, userID uuid.UUID, sub string, avatar *string) error {
	u := m.byID[userID]
	if u == nil {
		return nil
	}
	s := sub
	u.GoogleSub = &s
	m.bySub[sub] = u
	if avatar != nil && (u.AvatarURL == nil || *u.AvatarURL == "") {
		u.AvatarURL = avatar
	}
	return nil
}
func (m *memUsers) PutGoogleOAuthState(_ context.Context, st *model.GoogleOAuthState) error {
	cp := *st
	m.states[st.State] = &cp
	return nil
}
func (m *memUsers) ConsumeGoogleOAuthState(_ context.Context, state string) (*model.GoogleOAuthState, error) {
	st := m.states[state]
	delete(m.states, state)
	if st == nil || time.Now().After(st.ExpiresAt) {
		return nil, nil
	}
	cp := *st
	return &cp, nil
}
func (m *memUsers) PutGoogleOAuthTicket(_ context.Context, t *model.GoogleOAuthTicket) error {
	cp := *t
	m.tickets[t.Ticket] = &cp
	return nil
}
func (m *memUsers) ConsumeGoogleOAuthTicket(_ context.Context, ticket string) (*model.GoogleOAuthTicket, error) {
	t := m.tickets[ticket]
	delete(m.tickets, ticket)
	if t == nil || time.Now().After(t.ExpiresAt) {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

type memOrgs struct {
	bySlug map[string]*model.Organization
}

func (m *memOrgs) Create(_ context.Context, org *model.Organization) error {
	if m.bySlug == nil {
		m.bySlug = map[string]*model.Organization{}
	}
	org.CreatedAt = time.Now()
	cp := *org
	m.bySlug[org.Slug] = &cp
	return nil
}
func (m *memOrgs) FindByID(context.Context, uuid.UUID) (*model.Organization, error) {
	return nil, nil
}
func (m *memOrgs) FindBySlug(_ context.Context, slug string) (*model.Organization, error) {
	o := m.bySlug[slug]
	if o == nil {
		return nil, nil
	}
	cp := *o
	return &cp, nil
}
func (m *memOrgs) Update(context.Context, *model.Organization) error { return nil }
func (m *memOrgs) UpdateChart(context.Context, uuid.UUID, []model.UpdateOrgChartEntry) error {
	return nil
}

type memTokens struct{ n int }

func (m *memTokens) Save(context.Context, *model.RefreshToken) error {
	m.n++
	return nil
}
func (m *memTokens) FindByHash(context.Context, string) (*model.RefreshToken, error) {
	return nil, nil
}
func (m *memTokens) Delete(context.Context, uuid.UUID) error           { return nil }
func (m *memTokens) DeleteAllForUser(context.Context, uuid.UUID) error { return nil }

func testJWT(t *testing.T) JWTService {
	t.Helper()
	return NewJWTService(&config.Config{
		JWTSecret:            "test-secret-at-least-32-characters!",
		JWTExpiryMinutes:     15,
		JWTRefreshExpiryDays: 7,
	})
}

func googleSvc(t *testing.T, users repository.UserRepository, orgs repository.OrganizationRepository, tokens repository.TokenRepository, g googleauth.Identity) AuthService {
	t.Helper()
	cfg := googleauth.Config{
		ClientID:     "cid",
		ClientSecret: "csecret",
		RedirectURL:  "http://localhost/callback",
	}
	return NewAuthService(users, tokens, orgs, nil, nil, testJWT(t), g, cfg, zap.NewNop())
}

func TestGoogleSignupThenTicket(t *testing.T) {
	users := newMemUsers()
	orgs := &memOrgs{}
	tokens := &memTokens{}
	g := &fakeGoogle{
		url: "https://accounts.google.com/o/oauth2/v2/auth",
		profile: googleauth.Profile{
			Sub: "sub-1", Email: "alex@example.com", Name: "Alex Example",
			Picture: "https://example.com/a.png", EmailVerified: true,
		},
	}
	svc := googleSvc(t, users, orgs, tokens, g)

	authURL, err := svc.StartGoogle(context.Background(), "signup", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(authURL, "state=") {
		t.Fatalf("auth url missing state: %s", authURL)
	}
	var state string
	for s := range users.states {
		state = s
	}
	if state == "" {
		t.Fatal("expected stored state")
	}

	ticket, intent, err := svc.CompleteGoogle(context.Background(), state, "code")
	if err != nil {
		t.Fatal(err)
	}
	if intent != "signup" {
		t.Fatalf("intent %s", intent)
	}
	if ticket == "" {
		t.Fatal("empty ticket")
	}

	resp, err := svc.ExchangeGoogleTicket(context.Background(), ticket)
	if err != nil {
		t.Fatal(err)
	}
	if resp.User.Email != "alex@example.com" {
		t.Fatalf("email %s", resp.User.Email)
	}
	if resp.User.Role != "admin" {
		t.Fatalf("role %s", resp.User.Role)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("missing tokens")
	}
	if tokens.n != 1 {
		t.Fatalf("refresh tokens saved: %d", tokens.n)
	}
	if _, err := svc.ExchangeGoogleTicket(context.Background(), ticket); err != ErrInvalidGoogleTicket {
		t.Fatalf("ticket must be one-time, got %v", err)
	}

	// Second Google login finds the same user, does not create another org.
	authURL, err = svc.StartGoogle(context.Background(), "login", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = authURL
	for s := range users.states {
		state = s
	}
	ticket, _, err = svc.CompleteGoogle(context.Background(), state, "code")
	if err != nil {
		t.Fatal(err)
	}
	resp2, err := svc.ExchangeGoogleTicket(context.Background(), ticket)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.User.ID != resp.User.ID {
		t.Fatal("expected same user on second google login")
	}
	if len(orgs.bySlug) != 1 {
		t.Fatalf("expected one org, got %d", len(orgs.bySlug))
	}
}

func TestGoogleLinksExistingPasswordUser(t *testing.T) {
	users := newMemUsers()
	orgs := &memOrgs{}
	existing := &model.User{
		ID:       uuid.New(),
		Email:    "Alex@Example.com",
		Password: "hashed",
		FullName: "Alex",
		Role:     "admin",
	}
	_ = users.Create(context.Background(), existing)

	g := &fakeGoogle{
		url: "https://accounts.google.com",
		profile: googleauth.Profile{
			Sub: "sub-9", Email: "alex@example.com", Name: "Alex Example", EmailVerified: true,
		},
	}
	svc := googleSvc(t, users, orgs, &memTokens{}, g)
	_, err := svc.StartGoogle(context.Background(), "login", "")
	if err != nil {
		t.Fatal(err)
	}
	var state string
	for s := range users.states {
		state = s
	}
	ticket, _, err := svc.CompleteGoogle(context.Background(), state, "code")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := svc.ExchangeGoogleTicket(context.Background(), ticket)
	if err != nil {
		t.Fatal(err)
	}
	if resp.User.ID != existing.ID {
		t.Fatal("should link, not create")
	}
	got, _ := users.FindByGoogleSub(context.Background(), "sub-9")
	if got == nil {
		t.Fatal("google_sub not linked")
	}
	if len(orgs.bySlug) != 0 {
		t.Fatal("must not create an org when linking")
	}
}

func TestAbandonGoogleReturnsSignupIntent(t *testing.T) {
	users := newMemUsers()
	svc := googleSvc(t, users, &memOrgs{}, &memTokens{}, &fakeGoogle{url: "https://accounts.google.com"})
	_, err := svc.StartGoogle(context.Background(), "signup", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	var state string
	for s := range users.states {
		state = s
	}
	if got := svc.AbandonGoogle(context.Background(), state); got != "signup" {
		t.Fatalf("intent %s", got)
	}
	if _, ok := users.states[state]; ok {
		t.Fatal("state should be consumed")
	}
}

func TestGoogleInvalidState(t *testing.T) {
	svc := googleSvc(t, newMemUsers(), &memOrgs{}, &memTokens{}, &fakeGoogle{
		url:     "https://accounts.google.com",
		profile: googleauth.Profile{Sub: "x", Email: "a@b.com", EmailVerified: true},
	})
	_, _, err := svc.CompleteGoogle(context.Background(), "nope", "code")
	if err != ErrInvalidGoogleState {
		t.Fatalf("got %v", err)
	}
}

func TestGoogleNotConfigured(t *testing.T) {
	svc := NewAuthService(newMemUsers(), &memTokens{}, &memOrgs{}, nil, nil, testJWT(t), nil, googleauth.Config{}, zap.NewNop())
	if svc.GoogleEnabled() {
		t.Fatal("expected disabled")
	}
	_, err := svc.StartGoogle(context.Background(), "login", "")
	if err != ErrGoogleAuthNotConfigured {
		t.Fatalf("got %v", err)
	}
}

func TestGoogleOnlyUserCannotPasswordLogin(t *testing.T) {
	users := newMemUsers()
	sub := "sub-pw"
	u := &model.User{
		ID:        uuid.New(),
		Email:     "g@example.com",
		Password:  "",
		FullName:  "G",
		Role:      "admin",
		GoogleSub: &sub,
	}
	_ = users.Create(context.Background(), u)
	svc := googleSvc(t, users, &memOrgs{}, &memTokens{}, &fakeGoogle{url: "https://accounts.google.com"})
	_, err := svc.Login(context.Background(), model.LoginRequest{Email: "g@example.com", Password: "anything1"})
	if err != ErrInvalidCredentials {
		t.Fatalf("got %v", err)
	}
}

var (
	_ repository.UserRepository         = (*memUsers)(nil)
	_ repository.OrganizationRepository = (*memOrgs)(nil)
	_ repository.TokenRepository        = (*memTokens)(nil)
)
