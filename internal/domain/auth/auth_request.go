package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/windfall/uwu_service/pkg/errors"
)

// -------------------------------------------------------------------------
// Register Request
// -------------------------------------------------------------------------

type RegisterRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
	AvatarURL   string
}

func (req *RegisterRequest) ParseAndValidate(r *http.Request) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		return errors.Validation("invalid request body")
	}
	if req.Email == "" || req.Password == "" {
		return errors.Validation("email and password are required")
	}
	if len(req.Password) < 6 {
		return errors.Validation("password must be at least 6 characters")
	}

	// Generate random display name from email if not provided
	if req.DisplayName == "" {
		req.DisplayName = strings.Split(req.Email, "@")[0]
	}

	// Generate random avatar from email if not provided
	if req.AvatarURL == "" {
		req.AvatarURL = "https://api.dicebear.com/7.x/initials/svg?seed=" + req.DisplayName
	}

	return nil
}

// ToInput แปลงร่างจาก HTTP Request -> Service Input
func (req *RegisterRequest) ToInput() RegisterInput {
	return RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
		AvatarURL:   req.AvatarURL,
	}
}

// -------------------------------------------------------------------------
// Login Request
// -------------------------------------------------------------------------

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginInput struct {
	Email    string
	Password string
}

func (req *LoginRequest) ParseAndValidate(r *http.Request) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		return errors.Validation("invalid request body")
	}
	if req.Email == "" || req.Password == "" {
		return errors.Validation("email and password are required")
	}
	return nil
}

// ToInput แปลงร่างจาก HTTP Request -> Service Input
func (req *LoginRequest) ToInput() LoginInput {
	return LoginInput{
		Email:    req.Email,
		Password: req.Password,
	}
}
