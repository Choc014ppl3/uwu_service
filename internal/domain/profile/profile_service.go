package profile

import (
	"context"

	"github.com/windfall/uwu_service/pkg/errors"
)

// ProfileService handles profile operations.
type ProfileService struct {
	profileRepo ProfileRepository
}

// NewProfileService creates a new profile service.
func NewProfileService(profileRepo ProfileRepository) *ProfileService {
	return &ProfileService{
		profileRepo: profileRepo,
	}
}

// GetProfile returns the current user's profile.
func (s *ProfileService) GetProfile(ctx context.Context, userID string) (*Profile, *errors.AppError) {
	return s.profileRepo.GetProfile(ctx, userID)
}
