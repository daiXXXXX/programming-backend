package evaluator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/models"
	"github.com/robertkrimen/otto"
)

// Evaluator 代码评测器
type Evaluator struct {
	timeout time.Duration
}

// NewEvaluator 创建评测器
func NewEvaluator(timeout int) *Evaluator {
	return &Evaluator{
		timeout: time.Duration(timeout) * time.Millisecond,
	}
}

// EvaluateCode 评测代码
func (e *Evaluator) EvaluateCode(code string, testCases []models.TestCase) []models.TestResult {
	results := make([]models.TestResult, 0, len(testCases))

	for _, testCase := range testCases {
		result := e.runTestCase(code, testCase)
		results = append(results, result)
	}

	return results
}

// runTestCase 运行单个测试用例
func (e *Evaluator) runTestCase(code string, testCase models.TestCase) models.TestResult {
	result := models.TestResult{
		TestCaseID:     testCase.ID,
		Input:          testCase.Input,
		ExpectedOutput: testCase.ExpectedOutput,
		Passed:         false,
	}

	// 创建上下文以支持超时
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	// 使用 channel 来处理超时
	done := make(chan bool)
	startTime := time.Now()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				result.Error = fmt.Sprintf("Panic: %v", r)
			}
			done <- true
		}()

		// 执行代码
		vm := otto.New()
		vm.Interrupt = make(chan func(), 1) // 允许中断

		// 注入代码
		wrappedCode := fmt.Sprintf(`
			%s
			
			// 执行用户定义的函数
			var __result = (function() {
				if (typeof processInput === 'function') {
					return processInput(%s);
				}
				return null;
			})();
			
			__result;
		`, code, quoteString(testCase.Input))

		value, err := vm.Run(wrappedCode)
		if err != nil {
			result.Error = err.Error()
			return
		}

		// 获取输出
		output, err := value.ToString()
		if err != nil {
			result.Error = err.Error()
			return
		}

		result.ActualOutput = strings.TrimSpace(output)
		result.ExpectedOutput = strings.TrimSpace(testCase.ExpectedOutput)
		result.Passed = result.ActualOutput == result.ExpectedOutput
	}()

	// 等待执行完成或超时
	select {
	case <-ctx.Done():
		result.Error = "Time Limit Exceeded"
		result.ExecutionTime = int(e.timeout.Milliseconds())
	case <-done:
		result.ExecutionTime = int(time.Since(startTime).Milliseconds())
	}

	return result
}

// CalculateScore 计算分数
func (e *Evaluator) CalculateScore(results []models.TestResult) int {
	if len(results) == 0 {
		return 0
	}

	passed := 0
	for _, result := range results {
		if result.Passed {
			passed++
		}
	}

	return (passed * 100) / len(results)
}

// GetSubmissionStatus 获取提交状态
func (e *Evaluator) GetSubmissionStatus(results []models.TestResult) models.SubmissionStatus {
	hasError := false
	hasTimeLimitExceeded := false

	for _, result := range results {
		if result.Error != "" {
			if result.Error == "Time Limit Exceeded" {
				hasTimeLimitExceeded = true
			} else {
				hasError = true
			}
		}
	}

	if hasTimeLimitExceeded {
		return models.StatusTimeLimitExceeded
	}

	if hasError {
		return models.StatusRuntimeError
	}

	// 检查是否全部通过
	allPassed := true
	for _, result := range results {
		if !result.Passed {
			allPassed = false
			break
		}
	}

	if allPassed {
		return models.StatusAccepted
	}

	return models.StatusWrongAnswer
}

// quoteString 转义字符串用于JavaScript
func quoteString(s string) string {
	// 简单的字符串转义
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return `"` + s + `"`
}
