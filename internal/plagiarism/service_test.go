package plagiarism

import (
	"context"
	"testing"

	"github.com/daiXXXXX/programming-backend/internal/models"
)

// TestCheckClassProblemSelectsSuspiciousPair 验证高度相似的代码能被启发式筛选识别为可疑对。
func TestCheckClassProblemSelectsSuspiciousPair(t *testing.T) {
	service := NewService()
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

	if report.CandidatePairs != 1 {
		t.Fatalf("expected report candidate count to be 1, got %d", report.CandidatePairs)
	}

	if len(report.Results) != 1 {
		t.Fatalf("expected one result, got %d", len(report.Results))
	}

	// 启发式筛选后的结果应该标记为 suspicious
	if report.Results[0].Verdict != "suspicious" {
		t.Fatalf("expected suspicious verdict, got %q", report.Results[0].Verdict)
	}

	if report.Results[0].HeuristicScore < fixedMinHeuristic {
		t.Fatalf("expected heuristic score >= %.2f, got %.3f", fixedMinHeuristic, report.Results[0].HeuristicScore)
	}
}

// TestCheckClassProblemSkipsWhenFewerThanTwoSubmissions 验证少于两份提交时直接返回空结果。
func TestCheckClassProblemSkipsWhenFewerThanTwoSubmissions(t *testing.T) {
	service := NewService()

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

	if report.CandidatePairs != 0 {
		t.Fatalf("expected zero candidate pairs, got %d", report.CandidatePairs)
	}
}

// TestCheckClassProblemNoPairMeetsThreshold 验证不相似的代码不会被标记为可疑。
func TestCheckClassProblemNoPairMeetsThreshold(t *testing.T) {
	service := NewService()

	report, err := service.CheckClassProblem(context.Background(), 2, &models.Problem{
		ID:    88,
		Title: "Different Solutions",
	}, models.PlagiarismCheckRequest{
		ProblemID: 88,
	}, []models.ClassProblemSubmission{
		{
			UserID:       1,
			Username:     "alice",
			SubmissionID: 201,
			ProblemID:    88,
			Code:         "function solve(nums) { let sum = 0; for (let i = 0; i < nums.length; i++) { sum += nums[i]; } return sum; }",
			Language:     "JavaScript",
			Status:       models.StatusAccepted,
		},
		{
			UserID:       2,
			Username:     "bob",
			SubmissionID: 202,
			ProblemID:    88,
			Code:         "function solve(nums) { return nums.slice().sort((a, b) => a - b)[0] ?? -1; }",
			Language:     "JavaScript",
			Status:       models.StatusAccepted,
		},
	})
	if err != nil {
		t.Fatalf("CheckClassProblem returned error: %v", err)
	}

	if report.CandidatePairs != 0 {
		t.Fatalf("expected zero candidate pairs, got %d", report.CandidatePairs)
	}
}

// TestHeuristicSimilarityIgnoresSurfaceLevelChanges 验证变量改名等表面差异不影响启发式分数。
func TestHeuristicSimilarityIgnoresSurfaceLevelChanges(t *testing.T) {
	left := `
		// first version
		function solve(nums, target) {
			const cache = {};
			for (let i = 0; i < nums.length; i++) {
				const need = target - nums[i];
				if (cache[need] !== undefined) {
					return [cache[need], i];
				}
				cache[nums[i]] = i;
			}
			return [];
		}
	`

	right := `
		function solve(values, goal) {
			const seen = {};
			for (let index = 0; index < values.length; index++) {
				const remain = goal - values[index];
				if (seen[remain] !== undefined) {
					return [seen[remain], index];
				}
				seen[values[index]] = index;
			}
			return [];
		}
	`

	score := heuristicSimilarity(left, right)
	if score < 0.95 {
		t.Fatalf("expected surface-level rewrites to stay highly similar, got %.3f", score)
	}
}
