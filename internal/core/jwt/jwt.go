package jwt

import (
	"errors"
	"strconv"
	"time"

	"sonnda-api/internal/core/model"

	"github.com/golang-jwt/jwt/v5"
)

type JWTManager struct {
	Secret []byte
	Issuer string
	TTL    time.Duration
}

type Claims struct {
	UserID uint       `json:"uid"`
	Email  string     `json:"email"`
	Role   model.Role `json:"role"`
	jwt.RegisteredClaims
}

func NewJWTManager(secret, issuer string, ttl time.Duration) *JWTManager {
	return &JWTManager{
		Secret: []byte(secret),
		Issuer: issuer,
		TTL:    ttl,
	}
}

func (j *JWTManager) Generate(u *model.User) (string, error) {
	now := time.Now().UTC()
	claims := &Claims{
		UserID: u.ID,
		Email:  u.Email,
		Role:   u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.Issuer,
			Subject:   strconv.FormatUint(uint64(u.ID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.TTL)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(j.Secret)
}

func (j *JWTManager) Parse(tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return j.Secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid claims")
	}
	if claims.Issuer != j.Issuer {
		return nil, errors.New("invalid issuer")
	}
	return claims, nil
}
