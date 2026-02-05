// Package gemini provides a client for the Google Gemini generative AI API.
package gemini

import "strings"

// Request types

// Request represents a request to the Gemini generateContent endpoint.
type Request struct {
	Contents         []Content        `json:"contents"`
	GenerationConfig GenerationConfig `json:"generationConfig"`
	Tools            []Tool           `json:"tools,omitempty"`
}

// Content represents a content block containing parts.
type Content struct {
	Parts []Part `json:"parts"`
}

// Part represents a single part of a content block.
type Part struct {
	Text string `json:"text"`
}

// GenerationConfig controls generation parameters.
type GenerationConfig struct {
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
}

// Tool represents a tool available to the model.
type Tool struct {
	GoogleSearch *GoogleSearch `json:"googleSearch,omitempty"`
}

// GoogleSearch enables grounding with Google Search.
type GoogleSearch struct{}

// Response types

// Response represents the response from the Gemini generateContent endpoint.
type Response struct {
	Candidates    []Candidate   `json:"candidates"`
	UsageMetadata UsageMetadata `json:"usageMetadata"`
}

// Candidate represents a single generation candidate.
type Candidate struct {
	Content       ResponseContent `json:"content"`
	FinishReason  string          `json:"finishReason"`
	SafetyRatings []SafetyRating  `json:"safetyRatings"`
}

// ResponseContent represents the content of a candidate response.
type ResponseContent struct {
	Parts []ResponsePart `json:"parts"`
	Role  string         `json:"role"`
}

// ResponsePart represents a single part of a candidate response.
type ResponsePart struct {
	Text string `json:"text"`
}

// UsageMetadata contains token usage information.
type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// SafetyRating represents a safety rating for a candidate.
type SafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

// Text returns the concatenated text from all parts of the first candidate.
// Returns empty string if there are no candidates or parts.
func (r *Response) Text() string {
	if len(r.Candidates) == 0 {
		return ""
	}
	parts := r.Candidates[0].Content.Parts
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0].Text
	}
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}
