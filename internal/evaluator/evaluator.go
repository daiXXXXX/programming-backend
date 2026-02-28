package evaluator

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// EvaluateCode 评测代码（支持多语言）
func (e *Evaluator) EvaluateCode(code string, language string, testCases []models.TestCase) []models.TestResult {
	lang := strings.ToLower(strings.TrimSpace(language))

	switch lang {
	case "c":
		return e.evaluateC(code, testCases)
	default:
		// 默认 JavaScript
		return e.evaluateJS(code, testCases)
	}
}

// ==================== JavaScript 评测 ====================

func (e *Evaluator) evaluateJS(code string, testCases []models.TestCase) []models.TestResult {
	results := make([]models.TestResult, 0, len(testCases))
	for _, testCase := range testCases {
		result := e.runJSTestCase(code, testCase)
		results = append(results, result)
	}
	return results
}

func (e *Evaluator) runJSTestCase(code string, testCase models.TestCase) models.TestResult {
	result := models.TestResult{
		TestCaseID:     testCase.ID,
		Input:          testCase.Input,
		ExpectedOutput: testCase.ExpectedOutput,
		Passed:         false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	done := make(chan bool)
	startTime := time.Now()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				result.Error = fmt.Sprintf("Panic: %v", r)
			}
			done <- true
		}()

		vm := otto.New()
		vm.Interrupt = make(chan func(), 1)

		wrappedCode := fmt.Sprintf(`
			%s
			
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

		output, err := value.ToString()
		if err != nil {
			result.Error = err.Error()
			return
		}

		result.ActualOutput = strings.TrimSpace(output)
		result.ExpectedOutput = strings.TrimSpace(testCase.ExpectedOutput)
		result.Passed = result.ActualOutput == result.ExpectedOutput
	}()

	select {
	case <-ctx.Done():
		result.Error = "Time Limit Exceeded"
		result.ExecutionTime = int(e.timeout.Milliseconds())
	case <-done:
		result.ExecutionTime = int(time.Since(startTime).Milliseconds())
	}

	return result
}

// ==================== C 语言评测 ====================

func (e *Evaluator) evaluateC(code string, testCases []models.TestCase) []models.TestResult {
	results := make([]models.TestResult, 0, len(testCases))

	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "judge-c-*")
	if err != nil {
		for _, tc := range testCases {
			results = append(results, models.TestResult{
				TestCaseID:     tc.ID,
				Input:          tc.Input,
				ExpectedOutput: tc.ExpectedOutput,
				Error:          "System error: failed to create temp directory",
			})
		}
		return results
	}
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(tmpDir, "solution.c")
	binFile := filepath.Join(tmpDir, "solution")

	// 写入源码
	if err := os.WriteFile(srcFile, []byte(code), 0644); err != nil {
		for _, tc := range testCases {
			results = append(results, models.TestResult{
				TestCaseID:     tc.ID,
				Input:          tc.Input,
				ExpectedOutput: tc.ExpectedOutput,
				Error:          "System error: failed to write source file",
			})
		}
		return results
	}

	// 编译（限时 10 秒）
	compileCtx, compileCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer compileCancel()

	compileCmd := exec.CommandContext(compileCtx, "gcc", "-o", binFile, srcFile, "-lm", "-std=c11")
	var compileStderr bytes.Buffer
	compileCmd.Stderr = &compileStderr

	if err := compileCmd.Run(); err != nil {
		compileErr := strings.TrimSpace(compileStderr.String())
		if compileErr == "" {
			compileErr = err.Error()
		}
		// 编译失败，所有测试用例都标记为编译错误
		for _, tc := range testCases {
			results = append(results, models.TestResult{
				TestCaseID:     tc.ID,
				Input:          tc.Input,
				ExpectedOutput: tc.ExpectedOutput,
				Error:          fmt.Sprintf("Compilation Error: %s", compileErr),
			})
		}
		return results
	}

	// 逐个测试用例运行
	for _, testCase := range testCases {
		result := e.runCTestCase(binFile, testCase)
		results = append(results, result)
	}

	return results
}

func (e *Evaluator) runCTestCase(binFile string, testCase models.TestCase) models.TestResult {
	result := models.TestResult{
		TestCaseID:     testCase.ID,
		Input:          testCase.Input,
		ExpectedOutput: testCase.ExpectedOutput,
		Passed:         false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	startTime := time.Now()

	cmd := exec.CommandContext(ctx, binFile)
	cmd.Stdin = strings.NewReader(testCase.Input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result.ExecutionTime = int(time.Since(startTime).Milliseconds())

	if ctx.Err() == context.DeadlineExceeded {
		result.Error = "Time Limit Exceeded"
		result.ExecutionTime = int(e.timeout.Milliseconds())
		return result
	}

	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		result.Error = fmt.Sprintf("Runtime Error: %s", errMsg)
		return result
	}

	result.ActualOutput = strings.TrimSpace(stdout.String())
	result.ExpectedOutput = strings.TrimSpace(testCase.ExpectedOutput)
	result.Passed = result.ActualOutput == result.ExpectedOutput

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
