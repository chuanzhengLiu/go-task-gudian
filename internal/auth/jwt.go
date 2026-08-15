package auth

import (
	"ancient-texts-backend/internal/config"
	"ancient-texts-backend/internal/model"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID uint64      `json:"user_id"`
	Email  string      `json:"email"`
	Role   model.Role  `json:"role"`
	InstID *uint64     `json:"inst_id,omitempty"`
	jwt.RegisteredClaims
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

func GenerateTokens(user *model.User) (*TokenResponse, error) {
	now := time.Now()
	accessExpires := now.Add(time.Duration(config.AppConfig.TOKEN_TTL) * time.Second)
	refreshExpires := now.Add(time.Duration(config.AppConfig.REFRESH_TOKEN_TTL) * time.Second)

	accessClaims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		InstID: user.InstitutionID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExpires),
			Issuer:    "ancient-texts",
			Subject:   "access",
			ID:        uuid.New().String(),
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenStr, err := accessToken.SignedString([]byte(config.AppConfig.JWT_SECRET))
	if err != nil {
		return nil, err
	}

	refreshClaims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		InstID: user.InstitutionID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(refreshExpires),
			Issuer:    "ancient-texts",
			Subject:   "refresh",
			ID:        uuid.New().String(),
		},
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenStr, err := refreshToken.SignedString([]byte(config.AppConfig.JWT_REFRESH_SECRET))
	if err != nil {
		return nil, err
	}

	return &TokenResponse{
		AccessToken:  accessTokenStr,
		RefreshToken: refreshTokenStr,
		TokenType:    "Bearer",
		ExpiresIn:    config.AppConfig.TOKEN_TTL,
	}, nil
}

func ParseAccessToken(tokenStr string) (*Claims, error) {
	return parseToken(tokenStr, config.AppConfig.JWT_SECRET, "access")
}

func ParseRefreshToken(tokenStr string) (*Claims, error) {
	return parseToken(tokenStr, config.AppConfig.JWT_REFRESH_SECRET, "refresh")
}

func parseToken(tokenStr string, secret string, subject string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		if claims.Subject != subject {
			return nil, errors.New("invalid token subject")
		}
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
