package service

import (
	"context"

	"github.com/windfall/uwu_service/internal/repository"
)

type UserContentService struct {
	learningItemRepo repository.LearningItemRepository
}

func NewUserContentService(repo repository.LearningItemRepository) *UserContentService {
	return &UserContentService{learningItemRepo: repo}
}

func (s *UserContentService) GetContentsByFeature(ctx context.Context, featureID int, page, limit int) ([]repository.LearningItem, int, error) {
	return s.learningItemRepo.GetByFeatureID(ctx, featureID, page, limit)
}
