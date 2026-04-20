package plagiarism

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/daiXXXXX/programming-backend/internal/models"
)

// 启发式阈值固定为 0.55，不再暴露给外部调用者调整。
// 超过此阈值的 pair 直接作为可疑对返回给教师人工复核。
const (
	defaultMaxCandidates = 5
	maxCandidateLimit    = 10
	fixedMinHeuristic    = 0.55 // 固定阈值，不允许用户修改
	shingleSize          = 5
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

// Service 只做本地启发式相似度筛选，不再依赖外部 AI 服务。
// 超过固定阈值 (0.55) 的 pair 直接作为可疑对返回。
type Service struct{}

// NewService 创建查重服务（不再需要 AI client）。
func NewService() *Service {
	return &Service{}
}

// CheckClassProblem 对班级内某道题的学生提交做启发式相似度筛选。
// 超过固定阈值的 pair 作为可疑对直接返回给教师人工复核，不再调用 AI。
func (s *Service) CheckClassProblem(
	_ context.Context,
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
		report.OverallSummary = "题目元信息缺失，无法执行查重分析。"
		return report, nil
	}

	if len(submissions) < 2 {
		report.OverallSummary = "至少需要两份学生提交才能进行查重分析。"
		return report, nil
	}

	// 只使用固定阈值 0.55 做本地启发式筛选
	candidates := buildCandidatePairs(submissions, resolveMaxCandidates(req.MaxCandidates), fixedMinHeuristic)
	report.CandidatePairs = len(candidates)
	if len(candidates) == 0 {
		report.OverallSummary = "启发式筛选未发现超过阈值的可疑 pair。"
		return report, nil
	}

	report.OverallSummary = fmt.Sprintf("启发式筛选发现 %d 对可疑代码（阈值 %.2f），请教师人工复核。", len(candidates), fixedMinHeuristic)

	// 直接把通过启发式筛选的 pair 作为可疑对返回
	for _, candidate := range candidates {
		report.Results = append(report.Results, models.PlagiarismPairResult{
			PairKey:        candidate.PairKey,
			StudentA:       toReportStudent(candidate.Left),
			StudentB:       toReportStudent(candidate.Right),
			SubmissionA:    toSubmissionRef(candidate.Left),
			SubmissionB:    toSubmissionRef(candidate.Right),
			HeuristicScore: roundSimilarity(candidate.HeuristicScore),
			Verdict:        "suspicious",
			RiskLevel:      heuristicRiskLevel(candidate.HeuristicScore),
			Summary:        fmt.Sprintf("本地启发式相似度 %.1f%%，超过阈值 %.0f%%，建议人工复核。", candidate.HeuristicScore*100, fixedMinHeuristic*100),
			Evidence:       []string{"代码结构经标准化后高度相似（shingle Jaccard）"},
			Differences:    []string{},
			AlreadyMarked:  candidate.Left.MarkedCheating && candidate.Right.MarkedCheating,
		})
	}

	return report, nil
}

// heuristicRiskLevel 根据启发式分数确定风险等级
func heuristicRiskLevel(score float64) string {
	switch {
	case score >= 0.85:
		return "high"
	case score >= 0.70:
		return "medium"
	default:
		return "low"
	}
}

type candidatePair struct {
	PairKey        string
	Left           models.ClassProblemSubmission
	Right          models.ClassProblemSubmission
	HeuristicScore float64
}

func buildCandidatePairs(submissions []models.ClassProblemSubmission, maxCandidates int, minHeuristic float64) []candidatePair {
	var selected []candidatePair

	// 先在本地把所有提交两两比较一遍，只把真正分数较高、信号较强的
	// 少量候选 pair 送给成本更高的 AI 做进一步判定。
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
			if score >= minHeuristic {
				selected = append(selected, pair)
			}
		}
	}

	sortCandidates(selected)
	if len(selected) > maxCandidates {
		selected = selected[:maxCandidates]
	}

	return selected
}

func heuristicSimilarity(leftCode, rightCode string) float64 {
	left := normalizeSourceCode(leftCode)
	right := normalizeSourceCode(rightCode)

	if len(left) == 0 || len(right) == 0 {
		return 0
	}

	// 本地启发式分数故意保持简单，只比较标准化后的 token shingle，
	// 更细致的“是否真的抄袭”判断交给后续 AI 分析。
	return roundSimilarity(shingleJaccard(left, right, shingleSize))
}

func normalizeSourceCode(code string) []string {
	// 先去掉只影响表面形式的差异，避免变量改名、注释改写、字面量调整、
	// 格式变化这些因素过度影响本地相似度分数。
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

	tokens := tokenRegexp.FindAllString(normalized, -1)
	return normalizeTokens(tokens)
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
				// 把用户自定义标识符统一折叠成占位符，降低“只改变量名”
				// 这类表面改写对启发式分数的干扰。
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
		// 很短的代码也保留一个可比较片段，避免因为 token 数量太少
		// 直接被本地启发式完全忽略。
		result[strings.Join(tokens, " ")] = struct{}{}
		return result
	}

	for i := 0; i <= len(tokens)-size; i++ {
		result[strings.Join(tokens[i:i+size], " ")] = struct{}{}
	}
	return result
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

func toReportStudent(submission models.ClassProblemSubmission) models.PlagiarismStudent {
	return models.PlagiarismStudent{
		UserID:   submission.UserID,
		Username: submission.Username,
		Avatar:   submission.Avatar,
	}
}

func toSubmissionRef(submission models.ClassProblemSubmission) models.PlagiarismSubmissionRef {
	return models.PlagiarismSubmissionRef{
		ID:             submission.SubmissionID,
		Language:       submission.Language,
		Status:         submission.Status,
		Score:          submission.Score,
		SubmittedAt:    submission.SubmittedAt,
		Selection:      submission.Selection,
		Tags:           append([]string{}, submission.Tags...),
		MarkedCheating: submission.MarkedCheating,
	}
}

func roundSimilarity(value float64) float64 {
	if value < 0 {
		value = 0
	} else if value > 1 {
		value = 1
	}
	return float64(int(value*1000+0.5)) / 1000
}
