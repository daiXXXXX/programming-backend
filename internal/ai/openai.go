package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/config"
)

var ErrNotConfigured = errors.New("openai api key is not configured")

type PairAnalysisRequest struct {
	ProblemTitle       string          `json:"problemTitle"`
	ProblemDescription string          `json:"problemDescription,omitempty"`
	Pairs              []PairCandidate `json:"pairs"`
}

type PairCandidate struct {
	PairKey        string      `json:"pairKey"`
	HeuristicScore float64     `json:"heuristicScore"`
	Language       string      `json:"language"`
	StudentA       PairStudent `json:"studentA"`
	StudentB       PairStudent `json:"studentB"`
	CodeA          string      `json:"codeA"`
	CodeB          string      `json:"codeB"`
}

type PairStudent struct {
	UserID       int64     `json:"userId"`
	Username     string    `json:"username"`
	SubmissionID int64     `json:"submissionId"`
	Status       string    `json:"status"`
	SubmittedAt  time.Time `json:"submittedAt"`
	Selection    string    `json:"selection"`
}

type PairAnalysis struct {
	PairKey     string   `json:"pairKey"`
	Verdict     string   `json:"verdict"`
	RiskLevel   string   `json:"riskLevel"`
	Suspicious  bool     `json:"suspicious"`
	Confidence  float64  `json:"confidence"`
	Summary     string   `json:"summary"`
	Evidence    []string `json:"evidence"`
	Differences []string `json:"differences"`
}

type PairAnalysisResponse struct {
	OverallSummary string         `json:"overallSummary"`
	Analyses       []PairAnalysis `json:"analyses"`
}

type Client struct {
	apiKey          string
	baseURL         string
	model           string
	reasoningEffort string
	httpClient      *http.Client
}

func NewOpenAIClient(cfg config.OpenAIConfig) *Client {
	timeout := time.Duration(cfg.RequestTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	model := cfg.Model
	if strings.TrimSpace(model) == "" {
		model = "gpt-5-mini"
	}

	return &Client{
		apiKey:          strings.TrimSpace(cfg.APIKey),
		baseURL:         baseURL,
		model:           model,
		reasoningEffort: strings.TrimSpace(cfg.ReasoningEffort),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) AnalyzePairs(ctx context.Context, req *PairAnalysisRequest) (*PairAnalysisResponse, error) {
	if c == nil || c.apiKey == "" {
		return nil, ErrNotConfigured
	}

	payload, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal plagiarism payload: %w", err)
	}

	requestBody := openAIResponsesRequest{
		Model: c.model,
		Input: []openAIMessage{
			{
				Role: "system",
				Content: []openAIContent{
					{
						Type: "input_text",
						Text: plagiarismSystemPrompt,
					},
				},
			},
			{
				Role: "user",
				Content: []openAIContent{
					{
						Type: "input_text",
						Text: "Review the candidate code pairs below and return one structured judgment per pair.\n\n" + string(payload),
					},
				},
			},
		},
		Text: openAITextConfig{
			Format: openAIFormat{
				Type:        "json_schema",
				Name:        "plagiarism_review",
				Description: "Structured plagiarism review results for candidate code pairs",
				Strict:      true,
				Schema:      plagiarismOutputSchema(),
			},
		},
		MaxOutputTokens: 2500,
		Store:           false,
	}

	if c.reasoningEffort != "" {
		requestBody.Reasoning = &openAIReasoning{
			Effort: c.reasoningEffort,
		}
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal openai request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("build openai request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call openai responses api: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read openai response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("openai responses api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var envelope openAIResponsesEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}

	if envelope.Error != nil && envelope.Error.Message != "" {
		return nil, fmt.Errorf("openai error: %s", envelope.Error.Message)
	}

	rawJSON := extractOutputText(envelope.Output)
	if rawJSON == "" {
		return nil, errors.New("openai response did not contain structured output text")
	}

	var result PairAnalysisResponse
	if err := json.Unmarshal([]byte(rawJSON), &result); err != nil {
		return nil, fmt.Errorf("decode structured plagiarism result: %w", err)
	}

	for i := range result.Analyses {
		result.Analyses[i].Confidence = clamp01(result.Analyses[i].Confidence)
	}

	return &result, nil
}

const plagiarismSystemPrompt = `You are assisting a university instructor with plagiarism screening for programming assignments.

Rules:
- Each pair is from the same class and the same problem, so shared high-level algorithms are expected.
- Do not flag plagiarism just because both students used a standard or optimal solution.
- Focus on suspicious overlap that goes beyond the problem requirements: matching structure, uncommon helper decomposition, identical quirks, same redundant steps, same literal choices, same comment ideas, and similarity that survives variable renaming or formatting changes.
- Be conservative. False positives are expensive.
- Confidence must be a number between 0 and 1.
- verdict must be one of: unlikely, suspicious, likely_plagiarism.
- riskLevel must be one of: low, medium, high.
- suspicious should be true only when there is meaningful evidence of likely copying or excessive similarity.
- Keep summary concise and evidence-based.`

func plagiarismOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"overallSummary": map[string]any{
				"type": "string",
			},
			"analyses": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"pairKey": map[string]any{
							"type": "string",
						},
						"verdict": map[string]any{
							"type": "string",
							"enum": []string{"unlikely", "suspicious", "likely_plagiarism"},
						},
						"riskLevel": map[string]any{
							"type": "string",
							"enum": []string{"low", "medium", "high"},
						},
						"suspicious": map[string]any{
							"type": "boolean",
						},
						"confidence": map[string]any{
							"type": "number",
						},
						"summary": map[string]any{
							"type": "string",
						},
						"evidence": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
							},
						},
						"differences": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
							},
						},
					},
					"required": []string{
						"pairKey",
						"verdict",
						"riskLevel",
						"suspicious",
						"confidence",
						"summary",
						"evidence",
						"differences",
					},
				},
			},
		},
		"required": []string{"overallSummary", "analyses"},
	}
}

func extractOutputText(messages []openAIOutputMessage) string {
	var parts []string
	for _, message := range messages {
		for _, content := range message.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				parts = append(parts, content.Text)
			}
		}
	}
	return strings.Join(parts, "")
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

type openAIResponsesRequest struct {
	Model           string           `json:"model"`
	Input           []openAIMessage  `json:"input"`
	Text            openAITextConfig `json:"text"`
	Reasoning       *openAIReasoning `json:"reasoning,omitempty"`
	MaxOutputTokens int              `json:"max_output_tokens,omitempty"`
	Store           bool             `json:"store"`
}

type openAIMessage struct {
	Role    string          `json:"role"`
	Content []openAIContent `json:"content"`
}

type openAIContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAITextConfig struct {
	Format openAIFormat `json:"format"`
}

type openAIFormat struct {
	Type        string         `json:"type"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Strict      bool           `json:"strict,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
}

type openAIReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type openAIResponsesEnvelope struct {
	Error  *openAIErrorMessage   `json:"error"`
	Output []openAIOutputMessage `json:"output"`
}

type openAIErrorMessage struct {
	Message string `json:"message"`
}

type openAIOutputMessage struct {
	Content []openAIOutputContent `json:"content"`
}

type openAIOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
