package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTRepository struct
type jwtRepository struct {
	secret []byte
}

// NewJWTRepository constructor
func NewJWTRepository(jwtSecret string) *jwtRepository {
	return &jwtRepository{secret: []byte(jwtSecret)}
}

// ValidateToken parses and validates a JWT token string, returning the structured claims.
func (s *jwtRepository) ValidateToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	userID, ok := claims["sub"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid subject claim")
	}

	email, _ := claims["email"].(string)
	displayName, _ := claims["display_name"].(string)
	avatarURL, _ := claims["avatar_url"].(string)

	return &TokenClaims{
		UserID:      userID,
		Email:       email,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
	}, nil
}

func (s *jwtRepository) GenerateToken(user *User) (string, error) {
	claims := jwt.MapClaims{
		"sub":          user.ID.String(),
		"email":        user.Email,
		"display_name": user.DisplayName,
		"iat":          time.Now().Unix(),
		"exp":          time.Now().Add(72 * time.Hour).Unix(),
	}
	if user.AvatarURL != nil {
		claims["avatar_url"] = *user.AvatarURL
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}
