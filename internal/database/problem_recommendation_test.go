package database

import (
	"testing"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/models"
)

func TestScoreRecommendationCandidatePrefersRecentTagsAndDifficulty(t *testing.T) {
	profile := recommendationProfile{
		HasRecentAccepted: true,
		TagWeights: map[string]float64{
			"dp": 5,
		},
		DifficultyWeights: map[models.DifficultyLevel]float64{
			models.DifficultyMedium: 5,
		},
	}

	matchedScore, matchedTags := scoreRecommendationCandidate(models.Problem{
		Difficulty: models.DifficultyMedium,
		Tags:       []string{"DP", "array"},
	}, profile)
	otherScore, _ := scoreRecommendationCandidate(models.Problem{
		Difficulty: models.DifficultyHard,
		Tags:       []string{"graph"},
	}, profile)

	if matchedScore <= otherScore {
		t.Fatalf("expected candidate with matching tag and difficulty to score higher, got %.2f <= %.2f", matchedScore, otherScore)
	}
	if len(matchedTags) != 1 || matchedTags[0] != "DP" {
		t.Fatalf("expected DP to be returned as matched tag, got %#v", matchedTags)
	}
}

func TestScoreRecommendationCandidateColdStartPrefersEasyProblem(t *testing.T) {
	profile := recommendationProfile{
		TagWeights:        map[string]float64{},
		DifficultyWeights: map[models.DifficultyLevel]float64{},
	}

	easyScore, _ := scoreRecommendationCandidate(models.Problem{Difficulty: models.DifficultyEasy}, profile)
	hardScore, _ := scoreRecommendationCandidate(models.Problem{Difficulty: models.DifficultyHard}, profile)

	if easyScore <= hardScore {
		t.Fatalf("expected cold start to prefer easier problems, got %.2f <= %.2f", easyScore, hardScore)
	}
}

func TestDailyRecommendationTieChangesByDate(t *testing.T) {
	firstDay := dailyRecommendationTie(42, time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC))
	secondDay := dailyRecommendationTie(42, time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC))

	if firstDay == secondDay {
		t.Fatalf("expected tie breaker to vary by day, got %d", firstDay)
	}
}
