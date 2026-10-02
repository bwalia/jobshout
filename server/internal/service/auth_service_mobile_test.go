package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/appleauth"
	"github.com/jobshout/server/internal/googleauth"
	"github.com/jobshout/server/internal/model"
)

// memTokenStore is a refresh-token store that remembers, unlike memTokens,
// so refresh and logout can be exercised end to end.
type memTokenStore struct {
	byHash map[string]*model.RefreshToken
}

func newMemTokenStore() *memTokenStore {
	return &memTokenStore{byHash: map[string]*model.RefreshToken{}}
}

func (m *memTokenStore) Save(_ context.Context, t *model.RefreshToken) error {
	cp := *t
	m.byHash[t.TokenHash] = &cp
	return nil
}
func (m *memTokenStore) FindByHash(_ context.Context, h string) (*model.RefreshToken, error) {
	t := m.byHash[h]
	if t == nil {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}
func (m *memTokenStore) Delete(_ context.Context, id uuid.UUID) error {
	for h, t := range m.byHash {
		if t.ID == id {
			delete(m.byHash, h)
		}
	}
	return nil
}
func (m *memTokenStore) DeleteAllForUser(_ context.Context, userID uuid.UUID) error {
	for h, t := range m.byHash {
		if t.UserID == userID {
			delete(m.byHash, h)
		}
	}
	return nil
}

// deleteDevice mimics ON DELETE CASCADE from devices to refresh_tokens.
func (m *memTokenStore) deleteDevice(id uuid.UUID) {
	for h, t := range m.byHash {
		if t.DeviceID != nil && *t.DeviceID == id {
			delete(m.byHash, h)
		}
	}
}

type memDevices struct {
	byID   map[uuid.UUID]*model.Device
	tokens *memTokenStore
}

func newMemDevices(tokens *memTokenStore) *memDevices {
	return &memDevices{byID: map[uuid.UUID]*model.Device{}, tokens: tokens}
}

func (m *memDevices) Create(_ context.Context, d *model.Device) error {
	cp := *d
	m.byID[d.ID] = &cp
	return nil
}
func (m *memDevices) FindForUser(_ context.Context, id, userID uuid.UUID) (*model.Device, error) {
	d := m.byID[id]
	if d == nil || d.UserID != userID {
		return nil, nil
	}
	cp := *d
	return &cp, nil
}
func (m *memDevices) Touch(_ context.Context, d *model.Device) error {
	if cur := m.byID[d.ID]; cur != nil && cur.UserID == d.UserID {
		cur.Platform, cur.Name, cur.AppVersion = d.Platform, d.Name, d.AppVersion
	}
	return nil
}
func (m *memDevices) ListByUser(_ context.Context, userID uuid.UUID) ([]model.Device, error) {
	out := []model.Device{}
	for _, d := range m.byID {
		if d.UserID == userID {
			out = append(out, *d)
		}
	}
	return out, nil
}
func (m *memDevices) Delete(_ context.Context, id, userID uuid.UUID) (bool, error) {
	d := m.byID[id]
	if d == nil || d.UserID != userID {
		return false, nil
	}
	delete(m.byID, id)
	m.tokens.deleteDevice(id)
	return true, nil
}
func (m *memDevices) DeleteAllForUser(_ context.Context, userID uuid.UUID) error {
	for id, d := range m.byID {
		if d.UserID == userID {
			delete(m.byID, id)
			m.tokens.deleteDevice(id)
		}
	}
	return nil
}
func (m *memDevices) SetPush(_ context.Context, id, userID uuid.UUID, token, env string) (bool, error) {
	d := m.byID[id]
	if d == nil || d.UserID != userID {
		return false, nil
	}
	for _, other := range m.byID {
		if other.PushToken != nil && *other.PushToken == token {
			other.PushToken = nil
		}
	}
	if token == "" {
		d.PushToken, d.PushEnvironment = nil, nil
	} else {
		d.PushToken, d.PushEnvironment = &token, &env
	}
	return true, nil
}

type memIdentities struct{ links map[string]uuid.UUID }

func (m *memIdentities) FindUserID(_ context.Context, provider, subject string) (uuid.UUID, error) {
	return m.links[provider+"|"+subject], nil
}
func (m *memIdentities) Link(_ context.Context, provider, subject string, userID uuid.UUID, _ string) error {
	if _, ok := m.links[provider+"|"+subject]; !ok {
		m.links[provider+"|"+subject] = userID
	}
	return nil
}

type fakeApple struct {
	id  appleauth.Identity
	err error
	// gotNonce records the raw nonce the service passed through.
	gotNonce string
}

func (f *fakeApple) Verify(_ context.Context, _ string, rawNonce string) (appleauth.Identity, error) {
	f.gotNonce = rawNonce
	return f.id, f.err
}

type mobileFixture struct {
	svc        AuthService
	users      *memUsers
	tokens     *memTokenStore
	devices    *memDevices
	identities *memIdentities
	apple      *fakeApple
}

func newMobileFixture(t *testing.T) *mobileFixture {
	t.Helper()
	f := &mobileFixture{
		users:      newMemUsers(),
		tokens:     newMemTokenStore(),
		identities: &memIdentities{links: map[string]uuid.UUID{}},
		apple:      &fakeApple{},
	}
	f.devices = newMemDevices(f.tokens)
	f.svc = NewAuthService(f.users, f.tokens, &memOrgs{}, nil, nil, testJWT(t), nil, googleauth.Config{},
		MobileAuth{Devices: f.devices, Identities: f.identities, Apple: f.apple}, zap.NewNop())
	return f
}

func iphone() *model.DeviceInfo {
	return &model.DeviceInfo{Platform: "ios", Name: "iPhone", AppVersion: "1.0 (1)"}
}

func TestRegisterWithDeviceBindsRefreshAndSurvivesRotation(t *testing.T) {
	f := newMobileFixture(t)
	ctx := context.Background()

	resp, err := f.svc.Register(ctx, model.RegisterRequest{
		Email: "a@example.com", Password: "password1", FullName: "Ada", OrgName: "Ada Co", Device: iphone(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.DeviceID == nil {
		t.Fatal("expected a device id")
	}

	refreshed, err := f.svc.RefreshToken(ctx, resp.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.DeviceID == nil || *refreshed.DeviceID != *resp.DeviceID {
		t.Fatalf("rotated token lost its device: %v vs %v", refreshed.DeviceID, resp.DeviceID)
	}
}

func TestLoginReusesOwnDeviceButNotSomeoneElses(t *testing.T) {
	f := newMobileFixture(t)
	ctx := context.Background()

	a, _ := f.svc.Register(ctx, model.RegisterRequest{Email: "a@example.com", Password: "password1", FullName: "Ada", OrgName: "A", Device: iphone()})
	b, _ := f.svc.Register(ctx, model.RegisterRequest{Email: "b@example.com", Password: "password1", FullName: "Bea", OrgName: "B", Device: iphone()})

	again := iphone()
	again.ID = a.DeviceID.String()
	resp, err := f.svc.Login(ctx, model.LoginRequest{Email: "a@example.com", Password: "password1", Device: again})
	if err != nil {
		t.Fatal(err)
	}
	if *resp.DeviceID != *a.DeviceID {
		t.Fatal("own device id should be reused")
	}

	stolen := iphone()
	stolen.ID = a.DeviceID.String()
	resp, err = f.svc.Login(ctx, model.LoginRequest{Email: "b@example.com", Password: "password1", Device: stolen})
	if err != nil {
		t.Fatal(err)
	}
	if *resp.DeviceID == *a.DeviceID || *resp.DeviceID == *b.DeviceID {
		t.Fatal("another user's device id must not be adopted; expected a fresh one")
	}
}

func TestLogoutRevokesTheDeviceAndItsTokens(t *testing.T) {
	f := newMobileFixture(t)
	ctx := context.Background()

	resp, _ := f.svc.Register(ctx, model.RegisterRequest{Email: "a@example.com", Password: "password1", FullName: "Ada", OrgName: "A", Device: iphone()})
	if err := f.svc.Logout(ctx, resp.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.RefreshToken(ctx, resp.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("refresh after logout: got %v, want ErrInvalidRefreshToken", err)
	}
	if len(f.devices.byID) != 0 {
		t.Fatal("device should be gone after logout")
	}
	// Unknown tokens are not an error.
	if err := f.svc.Logout(ctx, "not-a-token"); err != nil {
		t.Fatal(err)
	}
}

func TestLogoutAllRevokesEverySession(t *testing.T) {
	f := newMobileFixture(t)
	ctx := context.Background()

	first, _ := f.svc.Register(ctx, model.RegisterRequest{Email: "a@example.com", Password: "password1", FullName: "Ada", OrgName: "A", Device: iphone()})
	web, _ := f.svc.Login(ctx, model.LoginRequest{Email: "a@example.com", Password: "password1"})

	if err := f.svc.LogoutAll(ctx, first.User.ID); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{first.RefreshToken, web.RefreshToken} {
		if _, err := f.svc.RefreshToken(ctx, tok); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Fatalf("token survived logout-all: %v", err)
		}
	}
}

func TestDevicePushIsScopedToOwner(t *testing.T) {
	f := newMobileFixture(t)
	ctx := context.Background()

	a, _ := f.svc.Register(ctx, model.RegisterRequest{Email: "a@example.com", Password: "password1", FullName: "Ada", OrgName: "A", Device: iphone()})
	b, _ := f.svc.Register(ctx, model.RegisterRequest{Email: "b@example.com", Password: "password1", FullName: "Bea", OrgName: "B", Device: iphone()})

	err := f.svc.SetDevicePush(ctx, b.User.ID, *a.DeviceID, model.DevicePushRequest{Token: "abc"})
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("setting push on someone else's device: got %v", err)
	}
	if err := f.svc.RevokeDevice(ctx, b.User.ID, *a.DeviceID); !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("revoking someone else's device: got %v", err)
	}

	if err := f.svc.SetDevicePush(ctx, a.User.ID, *a.DeviceID, model.DevicePushRequest{Token: "abc"}); err != nil {
		t.Fatal(err)
	}
	d := f.devices.byID[*a.DeviceID]
	if d.PushToken == nil || *d.PushToken != "abc" || d.PushEnvironment == nil || *d.PushEnvironment != "production" {
		t.Fatalf("push not stored with default environment: %+v", d)
	}
}

func TestSignInWithAppleCreatesThenFindsBySubject(t *testing.T) {
	f := newMobileFixture(t)
	ctx := context.Background()
	f.apple.id = appleauth.Identity{Subject: "001.apple", Email: "relay@privaterelay.appleid.com", EmailVerified: true, IsPrivateEmail: true}

	nonce, _, err := f.svc.AppleNonce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.svc.SignInWithApple(ctx, model.AppleSignInRequest{IdentityToken: "tok", Nonce: nonce, FullName: "Ada Apple", Device: iphone()})
	if err != nil {
		t.Fatal(err)
	}
	if f.apple.gotNonce != nonce {
		t.Fatal("raw nonce must reach the verifier")
	}
	if first.User.FullName != "Ada Apple" || first.DeviceID == nil {
		t.Fatalf("unexpected first sign-in: %+v", first)
	}

	// Later sign-ins carry no email; the linked subject must find the user.
	f.apple.id = appleauth.Identity{Subject: "001.apple"}
	nonce2, _, _ := f.svc.AppleNonce(ctx)
	second, err := f.svc.SignInWithApple(ctx, model.AppleSignInRequest{IdentityToken: "tok", Nonce: nonce2})
	if err != nil {
		t.Fatal(err)
	}
	if second.User.ID != first.User.ID {
		t.Fatal("second sign-in should resolve to the same user")
	}
}

func TestSignInWithAppleLinksExistingVerifiedEmail(t *testing.T) {
	f := newMobileFixture(t)
	ctx := context.Background()
	existing, _ := f.svc.Register(ctx, model.RegisterRequest{Email: "ada@example.com", Password: "password1", FullName: "Ada", OrgName: "A"})

	f.apple.id = appleauth.Identity{Subject: "002.apple", Email: "ada@example.com", EmailVerified: true}
	nonce, _, _ := f.svc.AppleNonce(ctx)
	resp, err := f.svc.SignInWithApple(ctx, model.AppleSignInRequest{IdentityToken: "tok", Nonce: nonce})
	if err != nil {
		t.Fatal(err)
	}
	if resp.User.ID != existing.User.ID {
		t.Fatal("verified email should link to the existing account")
	}
	if f.identities.links["apple|002.apple"] != existing.User.ID {
		t.Fatal("subject was not linked")
	}
}

func TestSignInWithAppleRejectsBadNonceAndMissingEmail(t *testing.T) {
	f := newMobileFixture(t)
	ctx := context.Background()
	f.apple.id = appleauth.Identity{Subject: "003.apple"}

	if _, err := f.svc.SignInWithApple(ctx, model.AppleSignInRequest{IdentityToken: "tok", Nonce: "never-issued-nonce"}); !errors.Is(err, ErrInvalidAppleToken) {
		t.Fatalf("unissued nonce: got %v", err)
	}

	nonce, _, _ := f.svc.AppleNonce(ctx)
	if _, err := f.svc.SignInWithApple(ctx, model.AppleSignInRequest{IdentityToken: "tok", Nonce: nonce}); !errors.Is(err, ErrAppleEmailRequired) {
		t.Fatalf("first sign-in without email: got %v", err)
	}
	// The nonce is single-use even when sign-in fails.
	if _, err := f.svc.SignInWithApple(ctx, model.AppleSignInRequest{IdentityToken: "tok", Nonce: nonce}); !errors.Is(err, ErrInvalidAppleToken) {
		t.Fatalf("reused nonce: got %v", err)
	}

	f.apple.err = appleauth.ErrInvalidToken
	nonce, _, _ = f.svc.AppleNonce(ctx)
	if _, err := f.svc.SignInWithApple(ctx, model.AppleSignInRequest{IdentityToken: "tok", Nonce: nonce}); !errors.Is(err, ErrInvalidAppleToken) {
		t.Fatalf("verifier rejection: got %v", err)
	}
}

func TestAppleDisabledWithoutVerifier(t *testing.T) {
	svc := NewAuthService(newMemUsers(), newMemTokenStore(), &memOrgs{}, nil, nil, testJWT(t), nil, googleauth.Config{}, MobileAuth{}, zap.NewNop())
	if svc.AppleEnabled() {
		t.Fatal("apple should be disabled")
	}
	if _, _, err := svc.AppleNonce(context.Background()); !errors.Is(err, ErrAppleAuthNotConfigured) {
		t.Fatalf("got %v", err)
	}
}
