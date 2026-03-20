package models

import "time"

type PlagiarismCheckRequest struct {
	ProblemID         int64   `json:"problemId" binding:"required"`
	AcceptedOnly      bool    `json:"acceptedOnly"`
	MaxCandidates     int     `json:"maxCandidates"`
	MinHeuristicScore float64 `json:"minHeuristicScore"`
}

type ClassProblemSubmission struct {
	UserID       int64            `json:"userId"`
	Username     string           `json:"username"`
	Avatar       string           `json:"avatar"`
	SubmissionID int64            `json:"submissionId"`
	ProblemID    int64            `json:"problemId"`
	Code         string           `json:"code"`
	Language     string           `json:"language"`
	Status       SubmissionStatus `json:"status"`
	Score        int              `json:"score"`
	SubmittedAt  time.Time        `json:"submittedAt"`
	Selection    string           `json:"selection"`
}

type PlagiarismStudent struct {
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

type PlagiarismSubmissionRef struct {
	ID          int64            `json:"id"`
	Language    string           `json:"language"`
	Status      SubmissionStatus `json:"status"`
	Score       int              `json:"score"`
	SubmittedAt time.Time        `json:"submittedAt"`
	Selection   string           `json:"selection"`
}

type PlagiarismPairResult struct {
	PairKey        string                  `json:"pairKey"`
	StudentA       PlagiarismStudent       `json:"studentA"`
	StudentB       PlagiarismStudent       `json:"studentB"`
	SubmissionA    PlagiarismSubmissionRef `json:"submissionA"`
	SubmissionB    PlagiarismSubmissionRef `json:"submissionB"`
	HeuristicScore float64                 `json:"heuristicScore"`
	AIConfidence   float64                 `json:"aiConfidence"`
	RiskLevel      string                  `json:"riskLevel"`
	Verdict        string                  `json:"verdict"`
	Summary        string                  `json:"summary"`
	Evidence       []string                `json:"evidence"`
	Differences    []string                `json:"differences"`
}

type PlagiarismCheckResponse struct {
	ClassID          int64                  `json:"classId"`
	ProblemID        int64                  `json:"problemId"`
	ProblemTitle     string                 `json:"problemTitle"`
	AcceptedOnly     bool                   `json:"acceptedOnly"`
	ComparedStudents int                    `json:"comparedStudents"`
	CandidatePairs   int                    `json:"candidatePairs"`
	OverallSummary   string                 `json:"overallSummary"`
	Results          []PlagiarismPairResult `json:"results"`
}
