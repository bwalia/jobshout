package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jobshout/server/internal/config"
)

// Claims represents the JWT payload for access tokens.
type Claims struct {
	UserID string `json:"sub"`
	Email  string `json:"email"`
	OrgID  string `json:"org_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// JWTService handles JWT token generation and validation.
type JWTService interface {
	GenerateAccessToken(userID uuid.UUID, email string, orgID *uuid.UUID, role string) (string, error)
	GenerateRefreshToken() (plain string, hash string, expiresAt time.Time)
	ParseAccessToken(tokenStr string) (*Claims, error)
	HashToken(token string) string
}

type jwtService struct {
	secret       []byte
	accessExpiry time.Duration
	refreshDays  int
}

// NewJWTService creates a new JWTService.
func NewJWTService(cfg *config.Config) JWTService {
	return &jwtService{
		secret:       []byte(cfg.JWTSecret),
		accessExpiry: cfg.AccessTokenExpiry(),
		refreshDays:  cfg.JWTRefreshExpiryDays,
	}
}

func (s *jwtService) GenerateAccessToken(userID uuid.UUID, email string, orgID *uuid.UUID, role string) (string, error) {
	orgIDStr := ""
	if orgID != nil {
		orgIDStr = orgID.String()
	}

	now := time.Now()
	claims := Claims{
		UserID: userID.String(),
		Email:  email,
		OrgID:  orgIDStr,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "jobshout",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("signing access token: %w", err)
	}
	return signedToken, nil
}

func (s *jwtService) GenerateRefreshToken() (string, string, time.Time) {
	plainToken := uuid.New().String()
	tokenHash := s.HashToken(plainToken)
	expiresAt := time.Now().Add(time.Duration(s.refreshDays) * 24 * time.Hour)
	return plainToken, tokenHash, expiresAt
}

func (s *jwtService) ParseAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parsing token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}

func (s *jwtService) HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
