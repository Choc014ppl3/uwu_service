package service

import (
	"context"

	"github.com/windfall/uwu_service/internal/repository"
)

type LearningSummaryData struct {
	NewWords           int `json:"new_words"`
	NewSentences       int `json:"new_sentences"`
	PassWords          int `json:"pass_words"`
	PassSentences      int `json:"pass_sentences"`
	RecognizeWords     int `json:"recognize_words"`
	RecognizeSentences int `json:"recognize_sentences"`
}

type GetLearningSummaryResp struct {
	Success bool                `json:"success"`
	Data    LearningSummaryData `json:"data"`
}

type UserStatsService struct {
	repo repository.UserStatsRepository
}

func NewUserStatsService(repo repository.UserStatsRepository) *UserStatsService {
	return &UserStatsService{repo: repo}
}

func (s *UserStatsService) GetLearningSummary(ctx context.Context, userID, language string, statuses []string) (*GetLearningSummaryResp, error) {
	summary, err := s.repo.GetLearningSummary(ctx, userID, language, statuses)
	if err != nil {
		return nil, err
	}

	if summary == nil {
		summary = &repository.LearningSummary{}
	}

	data := LearningSummaryData{
		NewWords:           summary.NewWords,
		NewSentences:       summary.NewSentences,
		PassWords:          summary.PassWords,
		PassSentences:      summary.PassSentences,
		RecognizeWords:     summary.RecognizeWords,
		RecognizeSentences: summary.RecognizeSentences,
	}

	return &GetLearningSummaryResp{Success: true, Data: data}, nil
}
