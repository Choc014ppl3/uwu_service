package auth

import (
	"encoding/json"
	"errors"
	"net/http"
)

// -------------------------------------------------------------------------
// Register Request
// -------------------------------------------------------------------------

type RegisterRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"name"`
}

func (req *RegisterRequest) ParseAndValidate(r *http.Request) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		return errors.New("invalid request body")
	}
	if req.Email == "" || req.Password == "" {
		return errors.New("email and password are required")
	}
	if len(req.Password) < 6 {
		return errors.New("password must be at least 6 characters")
	}
	return nil
}

// ToDTO แปลงร่างจาก HTTP Request -> Service Input
func (req *RegisterRequest) ToDTO() RegisterInput {
	return RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
	}
}

// -------------------------------------------------------------------------
// Login Request
// -------------------------------------------------------------------------

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (req *LoginRequest) ParseAndValidate(r *http.Request) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		return errors.New("invalid request body")
	}
	if req.Email == "" || req.Password == "" {
		return errors.New("email and password are required")
	}
	return nil
}

// ToDTO แปลงร่างจาก HTTP Request -> Service Input
func (req *LoginRequest) ToDTO() LoginInput {
	return LoginInput{
		Email:    req.Email,
		Password: req.Password,
	}
}
