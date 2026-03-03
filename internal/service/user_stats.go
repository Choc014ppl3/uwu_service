package service

import (
	"context"

	"github.com/windfall/uwu_service/internal/repository"
)

type LearningSummaryData struct {
	Type          string `json:"type"`
	NewWords      int    `json:"new_words"`
	NewSentences  int    `json:"new_sentences"`
	PassWords     int    `json:"pass_words"`
	PassSentences int    `json:"pass_sentences"`
}

type GetLearningSummaryResp struct {
	Success bool                  `json:"success"`
	Data    []LearningSummaryData `json:"data"`
}

type UserStatsService struct {
	repo repository.UserStatsRepository
}

func NewUserStatsService(repo repository.UserStatsRepository) *UserStatsService {
	return &UserStatsService{repo: repo}
}

func (s *UserStatsService) GetLearningSummary(ctx context.Context, userID, language string) (*GetLearningSummaryResp, error) {
	summary, err := s.repo.GetLearningSummary(ctx, userID, language)
	if err != nil {
		return nil, err
	}

	types := []string{"listen", "read", "write", "speak"}
	var data []LearningSummaryData

	if summary == nil {
		summary = &repository.LearningSummary{}
	}

	for _, t := range types {
		data = append(data, LearningSummaryData{
			Type:          t,
			NewWords:      summary.NewWords,
			NewSentences:  summary.NewSentences,
			PassWords:     summary.PassWords,
			PassSentences: summary.PassSentences,
		})
	}

	return &GetLearningSummaryResp{Success: true, Data: data}, nil
}
