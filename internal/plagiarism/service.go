package plagiarism

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/daiXXXXX/programming-backend/internal/ai"
	"github.com/daiXXXXX/programming-backend/internal/models"
)

var ErrAnalyzerNotConfigured = errors.New("ai plagiarism analyzer is not configured")

const (
	defaultMaxCandidates   = 5
	maxCandidateLimit      = 10
	defaultMinHeuristic    = 0.55
	fallbackMinHeuristic   = 0.40
	maxProblemDescription  = 1600
	maxCodeCharactersForAI = 7000
	shingleSize            = 5
)

var (
	doubleQuotedStringRegexp = regexp.MustCompile(`"(\\.|[^"\\])*"`)
	singleQuotedStringRegexp = regexp.MustCompile(`'(\\.|[^'\\])*'`)
	backtickStringRegexp     = regexp.MustCompile("`[^`]*`")
	blockCommentRegexp       = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineCommentRegexp        = regexp.MustCompile(`(?m)//.*$`)
	hashCommentRegexp        = regexp.MustCompile(`(?m)#.*$`)
	numberRegexp             = regexp.MustCompile(`\b\d+(\.\d+)?\b`)
	tokenRegexp              = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*|==|!=|<=|>=|&&|\|\||[{}()\[\];,.:+\-*/%<>!=]`)
	whitespaceRegexp         = regexp.MustCompile(`\s+`)
	identifierRegexp         = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

var keywordSet = map[string]struct{}{
	"and": {}, "as": {}, "break": {}, "case": {}, "catch": {}, "class": {}, "const": {}, "continue": {},
	"default": {}, "def": {}, "do": {}, "elif": {}, "else": {}, "enum": {}, "export": {}, "extends": {},
	"false": {}, "finally": {}, "for": {}, "from": {}, "func": {}, "function": {}, "if": {}, "import": {},
	"in": {}, "interface": {}, "let": {}, "new": {}, "nil": {}, "null": {}, "package": {}, "private": {},
	"protected": {}, "public": {}, "range": {}, "return": {}, "static": {}, "struct": {}, "super": {},
	"switch": {}, "this": {}, "throw": {}, "true": {}, "try": {}, "type": {}, "var": {}, "void": {},
	"while": {},
}

type analysisClient interface {
	AnalyzePairs(ctx context.Context, req *ai.PairAnalysisRequest) (*ai.PairAnalysisResponse, error)
}

type Service struct {
	client analysisClient
}

func NewService(client analysisClient) *Service {
	return &Service{client: client}
}

func (s *Service) CheckClassProblem(
	ctx context.Context,
	classID int64,
	problem *models.Problem,
	req models.PlagiarismCheckRequest,
	submissions []models.ClassProblemSubmission,
) (*models.PlagiarismCheckResponse, error) {
	report := &models.PlagiarismCheckResponse{
		ClassID:          classID,
		ProblemID:        req.ProblemID,
		AcceptedOnly:     req.AcceptedOnly,
		ComparedStudents: len(submissions),
		Results:          []models.PlagiarismPairResult{},
	}

	if problem != nil {
		report.ProblemTitle = problem.Title
	}

	if problem == nil {
		report.OverallSummary = "Problem metadata is missing, so plagiarism analysis could not run."
		return report, nil
	}

	if len(submissions) < 2 {
		report.OverallSummary = "At least two student submissions are required before plagiarism analysis can run."
		return report, nil
	}

	candidates := buildCandidatePairs(submissions, resolveMaxCandidates(req.MaxCandidates), resolveMinHeuristic(req.MinHeuristicScore))
	report.CandidatePairs = len(candidates)
	if len(candidates) == 0 {
		report.OverallSummary = "Heuristic pre-screen found no suspicious pairs worth sending to the AI reviewer."
		return report, nil
	}

	if s == nil || s.client == nil {
		return nil, ErrAnalyzerNotConfigured
	}

	analysisRequest := buildAnalysisRequest(problem, candidates)
	analysisResponse, err := s.client.AnalyzePairs(ctx, analysisRequest)
	if err != nil {
		if errors.Is(err, ai.ErrNotConfigured) {
			return nil, ErrAnalyzerNotConfigured
		}
		return nil, fmt.Errorf("analyze candidate pairs: %w", err)
	}

	report.OverallSummary = strings.TrimSpace(analysisResponse.OverallSummary)
	if report.OverallSummary == "" {
		report.OverallSummary = "AI analysis completed for the candidate pairs."
	}

	analysisByKey := make(map[string]ai.PairAnalysis, len(analysisResponse.Analyses))
	for _, analysis := range analysisResponse.Analyses {
		analysisByKey[analysis.PairKey] = analysis
	}

	for _, candidate := range candidates {
		analysis, ok := analysisByKey[candidate.PairKey]
		if !ok {
			analysis = ai.PairAnalysis{
				PairKey:     candidate.PairKey,
				Verdict:     "needs_review",
				RiskLevel:   "medium",
				Confidence:  candidate.HeuristicScore,
				Summary:     "The candidate was selected by the heuristic pre-screen, but the AI response did not include a structured verdict for it.",
				Evidence:    []string{"Local similarity pre-screen selected this pair."},
				Differences: []string{},
			}
		}

		report.Results = append(report.Results, models.PlagiarismPairResult{
			PairKey:        candidate.PairKey,
			StudentA:       toReportStudent(candidate.Left),
			StudentB:       toReportStudent(candidate.Right),
			SubmissionA:    toSubmissionRef(candidate.Left),
			SubmissionB:    toSubmissionRef(candidate.Right),
			HeuristicScore: roundSimilarity(candidate.HeuristicScore),
			AIConfidence:   roundSimilarity(clamp01(analysis.Confidence)),
			RiskLevel:      strings.TrimSpace(analysis.RiskLevel),
			Verdict:        strings.TrimSpace(analysis.Verdict),
			Summary:        strings.TrimSpace(analysis.Summary),
			Evidence:       normalizeList(analysis.Evidence),
			Differences:    normalizeList(analysis.Differences),
		})
	}

	return report, nil
}

type candidatePair struct {
	PairKey        string
	Left           models.ClassProblemSubmission
	Right          models.ClassProblemSubmission
	HeuristicScore float64
}

type normalizedSource struct {
	Tokens []string
	Lines  []string
}

func buildCandidatePairs(submissions []models.ClassProblemSubmission, maxCandidates int, minHeuristic float64) []candidatePair {
	var allPairs []candidatePair
	var selected []candidatePair

	for i := 0; i < len(submissions); i++ {
		for j := i + 1; j < len(submissions); j++ {
			left, right := orderedPair(submissions[i], submissions[j])
			if !comparableLanguages(left.Language, right.Language) {
				continue
			}

			score := heuristicSimilarity(left.Code, right.Code)
			pair := candidatePair{
				PairKey:        makePairKey(left.UserID, right.UserID),
				Left:           left,
				Right:          right,
				HeuristicScore: score,
			}
			allPairs = append(allPairs, pair)
			if score >= minHeuristic {
				selected = append(selected, pair)
			}
		}
	}

	sortCandidates(selected)
	if len(selected) == 0 {
		sortCandidates(allPairs)
		if len(allPairs) > 0 && allPairs[0].HeuristicScore >= fallbackMinHeuristic {
			selected = append(selected, allPairs[0])
		}
	}

	if len(selected) > maxCandidates {
		selected = selected[:maxCandidates]
	}

	return selected
}

func buildAnalysisRequest(problem *models.Problem, candidates []candidatePair) *ai.PairAnalysisRequest {
	request := &ai.PairAnalysisRequest{
		ProblemTitle:       problem.Title,
		ProblemDescription: trimForAI(problem.Description, maxProblemDescription),
		Pairs:              make([]ai.PairCandidate, 0, len(candidates)),
	}

	for _, candidate := range candidates {
		request.Pairs = append(request.Pairs, ai.PairCandidate{
			PairKey:        candidate.PairKey,
			HeuristicScore: roundSimilarity(candidate.HeuristicScore),
			Language:       preferredLanguage(candidate.Left.Language, candidate.Right.Language),
			StudentA:       toAIPairStudent(candidate.Left),
			StudentB:       toAIPairStudent(candidate.Right),
			CodeA:          trimForAI(candidate.Left.Code, maxCodeCharactersForAI),
			CodeB:          trimForAI(candidate.Right.Code, maxCodeCharactersForAI),
		})
	}

	return request
}

func heuristicSimilarity(leftCode, rightCode string) float64 {
	left := normalizeSourceCode(leftCode)
	right := normalizeSourceCode(rightCode)

	if len(left.Tokens) == 0 || len(right.Tokens) == 0 {
		return 0
	}

	tokenScore := shingleJaccard(left.Tokens, right.Tokens, shingleSize)
	lineScore := setJaccard(left.Lines, right.Lines)
	lengthScore := lengthBalance(len(left.Tokens), len(right.Tokens))

	return roundSimilarity(clamp01(0.65*tokenScore + 0.25*lineScore + 0.10*lengthScore))
}

func normalizeSourceCode(code string) normalizedSource {
	normalized := strings.ReplaceAll(code, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = doubleQuotedStringRegexp.ReplaceAllString(normalized, "STR")
	normalized = singleQuotedStringRegexp.ReplaceAllString(normalized, "STR")
	normalized = backtickStringRegexp.ReplaceAllString(normalized, "STR")
	normalized = blockCommentRegexp.ReplaceAllString(normalized, "\n")
	normalized = lineCommentRegexp.ReplaceAllString(normalized, "")
	normalized = hashCommentRegexp.ReplaceAllString(normalized, "")
	normalized = numberRegexp.ReplaceAllString(normalized, "NUM")
	normalized = strings.ToLower(normalized)

	var lines []string
	for _, line := range strings.Split(normalized, "\n") {
		line = whitespaceRegexp.ReplaceAllString(strings.TrimSpace(line), " ")
		if line == "" {
			continue
		}

		tokens := tokenRegexp.FindAllString(line, -1)
		if len(tokens) == 0 {
			continue
		}
		lines = append(lines, strings.Join(normalizeTokens(tokens), " "))
	}

	tokens := tokenRegexp.FindAllString(normalized, -1)
	return normalizedSource{
		Tokens: normalizeTokens(tokens),
		Lines:  lines,
	}
}

func normalizeTokens(tokens []string) []string {
	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(strings.ToLower(token))
		if token == "" {
			continue
		}
		if identifierRegexp.MatchString(token) {
			if _, isKeyword := keywordSet[token]; !isKeyword {
				token = "id"
			}
		}
		result = append(result, token)
	}
	return result
}

func shingleJaccard(left, right []string, size int) float64 {
	leftSet := buildShingles(left, size)
	rightSet := buildShingles(right, size)
	if len(leftSet) == 0 || len(rightSet) == 0 {
		return 0
	}

	intersection := 0
	union := make(map[string]struct{}, len(leftSet)+len(rightSet))
	for shingle := range leftSet {
		union[shingle] = struct{}{}
	}
	for shingle := range rightSet {
		if _, exists := leftSet[shingle]; exists {
			intersection++
		}
		union[shingle] = struct{}{}
	}

	if len(union) == 0 {
		return 0
	}
	return float64(intersection) / float64(len(union))
}

func buildShingles(tokens []string, size int) map[string]struct{} {
	result := make(map[string]struct{})
	if len(tokens) == 0 {
		return result
	}

	if len(tokens) <= size {
		result[strings.Join(tokens, " ")] = struct{}{}
		return result
	}

	for i := 0; i <= len(tokens)-size; i++ {
		result[strings.Join(tokens[i:i+size], " ")] = struct{}{}
	}
	return result
}

func setJaccard(left, right []string) float64 {
	leftSet := make(map[string]struct{}, len(left))
	rightSet := make(map[string]struct{}, len(right))
	for _, item := range left {
		leftSet[item] = struct{}{}
	}
	for _, item := range right {
		rightSet[item] = struct{}{}
	}

	if len(leftSet) == 0 || len(rightSet) == 0 {
		return 0
	}

	intersection := 0
	union := make(map[string]struct{}, len(leftSet)+len(rightSet))
	for item := range leftSet {
		union[item] = struct{}{}
	}
	for item := range rightSet {
		if _, exists := leftSet[item]; exists {
			intersection++
		}
		union[item] = struct{}{}
	}

	if len(union) == 0 {
		return 0
	}
	return float64(intersection) / float64(len(union))
}

func lengthBalance(leftLen, rightLen int) float64 {
	if leftLen == 0 || rightLen == 0 {
		return 0
	}

	smaller := leftLen
	larger := rightLen
	if smaller > larger {
		smaller, larger = larger, smaller
	}
	return float64(smaller) / float64(larger)
}

func orderedPair(left, right models.ClassProblemSubmission) (models.ClassProblemSubmission, models.ClassProblemSubmission) {
	if left.UserID <= right.UserID {
		return left, right
	}
	return right, left
}

func comparableLanguages(left, right string) bool {
	left = strings.TrimSpace(strings.ToLower(left))
	right = strings.TrimSpace(strings.ToLower(right))
	return left == "" || right == "" || left == right
}

func preferredLanguage(left, right string) string {
	if strings.TrimSpace(left) != "" {
		return left
	}
	return right
}

func makePairKey(leftUserID, rightUserID int64) string {
	return fmt.Sprintf("%d:%d", leftUserID, rightUserID)
}

func sortCandidates(candidates []candidatePair) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].HeuristicScore == candidates[j].HeuristicScore {
			return candidates[i].PairKey < candidates[j].PairKey
		}
		return candidates[i].HeuristicScore > candidates[j].HeuristicScore
	})
}

func resolveMaxCandidates(value int) int {
	switch {
	case value <= 0:
		return defaultMaxCandidates
	case value > maxCandidateLimit:
		return maxCandidateLimit
	default:
		return value
	}
}

func resolveMinHeuristic(value float64) float64 {
	if value <= 0 || value >= 1 {
		return defaultMinHeuristic
	}
	return value
}

func toReportStudent(submission models.ClassProblemSubmission) models.PlagiarismStudent {
	return models.PlagiarismStudent{
		UserID:   submission.UserID,
		Username: submission.Username,
		Avatar:   submission.Avatar,
	}
}

func toSubmissionRef(submission models.ClassProblemSubmission) models.PlagiarismSubmissionRef {
	return models.PlagiarismSubmissionRef{
		ID:          submission.SubmissionID,
		Language:    submission.Language,
		Status:      submission.Status,
		Score:       submission.Score,
		SubmittedAt: submission.SubmittedAt,
		Selection:   submission.Selection,
	}
}

func toAIPairStudent(submission models.ClassProblemSubmission) ai.PairStudent {
	return ai.PairStudent{
		UserID:       submission.UserID,
		Username:     submission.Username,
		SubmissionID: submission.SubmissionID,
		Status:       string(submission.Status),
		SubmittedAt:  submission.SubmittedAt,
		Selection:    submission.Selection,
	}
}

func trimForAI(value string, maxChars int) string {
	value = strings.TrimSpace(value)
	if maxChars <= 0 || len(value) <= maxChars {
		return value
	}
	return value[:maxChars] + "\n...[truncated]"
}

func normalizeList(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}

	if result == nil {
		return []string{}
	}
	return result
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func roundSimilarity(value float64) float64 {
	value = clamp01(value)
	return float64(int(value*1000+0.5)) / 1000
}
