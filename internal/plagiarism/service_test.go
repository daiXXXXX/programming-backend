package plagiarism

import (
	"context"
	"testing"

	"github.com/daiXXXXX/programming-backend/internal/ai"
	"github.com/daiXXXXX/programming-backend/internal/models"
)

type fakeAnalyzer struct {
	called bool
	req    *ai.PairAnalysisRequest
	resp   *ai.PairAnalysisResponse
	err    error
}

func (f *fakeAnalyzer) AnalyzePairs(_ context.Context, req *ai.PairAnalysisRequest) (*ai.PairAnalysisResponse, error) {
	f.called = true
	f.req = req
	return f.resp, f.err
}

func TestCheckClassProblemSelectsSuspiciousPair(t *testing.T) {
	analyzer := &fakeAnalyzer{
		resp: &ai.PairAnalysisResponse{
			OverallSummary: "One suspicious pair found.",
			Analyses: []ai.PairAnalysis{
				{
					PairKey:     "1:2",
					Verdict:     "likely_plagiarism",
					RiskLevel:   "high",
					Suspicious:  true,
					Confidence:  0.93,
					Summary:     "Both solutions share the same uncommon helper structure and redundant branches.",
					Evidence:    []string{"Matching helper decomposition", "Same redundant branch ordering"},
					Differences: []string{"Variable names were renamed"},
				},
			},
		},
	}

	service := NewService(analyzer)
	problem := &models.Problem{
		ID:          7,
		Title:       "Two Sum",
		Description: "Find two numbers that add up to the target.",
	}

	report, err := service.CheckClassProblem(context.Background(), 3, problem, models.PlagiarismCheckRequest{
		ProblemID: 7,
	}, []models.ClassProblemSubmission{
		{
			UserID:       1,
			Username:     "alice",
			SubmissionID: 101,
			ProblemID:    7,
			Code:         "function solve(nums, target) { const map = {}; for (let i = 0; i < nums.length; i++) { const need = target - nums[i]; if (map[need] !== undefined) { return [map[need], i]; } map[nums[i]] = i; } return []; }",
			Language:     "JavaScript",
			Status:       models.StatusAccepted,
			Selection:    "latest_accepted",
		},
		{
			UserID:       2,
			Username:     "bob",
			SubmissionID: 102,
			ProblemID:    7,
			Code:         "function solve(arr, target) { const cache = {}; for (let idx = 0; idx < arr.length; idx++) { const rest = target - arr[idx]; if (cache[rest] !== undefined) { return [cache[rest], idx]; } cache[arr[idx]] = idx; } return []; }",
			Language:     "JavaScript",
			Status:       models.StatusAccepted,
			Selection:    "latest_accepted",
		},
		{
			UserID:       3,
			Username:     "carol",
			SubmissionID: 103,
			ProblemID:    7,
			Code:         "function solve(nums, target) { nums.sort((a, b) => a - b); return nums.length ? [0, 0] : []; }",
			Language:     "JavaScript",
			Status:       models.StatusAccepted,
			Selection:    "latest_accepted",
		},
	})
	if err != nil {
		t.Fatalf("CheckClassProblem returned error: %v", err)
	}

	if !analyzer.called {
		t.Fatalf("expected analyzer to be called")
	}

	if len(analyzer.req.Pairs) != 1 {
		t.Fatalf("expected exactly one candidate pair, got %d", len(analyzer.req.Pairs))
	}

	if report.CandidatePairs != 1 {
		t.Fatalf("expected report candidate count to be 1, got %d", report.CandidatePairs)
	}

	if len(report.Results) != 1 {
		t.Fatalf("expected one result, got %d", len(report.Results))
	}

	if report.Results[0].Verdict != "likely_plagiarism" {
		t.Fatalf("expected likely_plagiarism verdict, got %q", report.Results[0].Verdict)
	}
}

func TestCheckClassProblemSkipsAIWhenFewerThanTwoSubmissions(t *testing.T) {
	analyzer := &fakeAnalyzer{}
	service := NewService(analyzer)

	report, err := service.CheckClassProblem(context.Background(), 1, &models.Problem{
		ID:    99,
		Title: "Example",
	}, models.PlagiarismCheckRequest{
		ProblemID: 99,
	}, []models.ClassProblemSubmission{
		{
			UserID:       1,
			Username:     "solo",
			SubmissionID: 1,
			ProblemID:    99,
			Code:         "return 1;",
			Language:     "JavaScript",
			Status:       models.StatusAccepted,
		},
	})
	if err != nil {
		t.Fatalf("CheckClassProblem returned error: %v", err)
	}

	if analyzer.called {
		t.Fatalf("analyzer should not be called when there are fewer than two submissions")
	}

	if report.CandidatePairs != 0 {
		t.Fatalf("expected zero candidate pairs, got %d", report.CandidatePairs)
	}
}
