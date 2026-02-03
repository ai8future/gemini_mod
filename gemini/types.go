package gemini

// Request types

type Request struct {
	Contents         []Content        `json:"contents"`
	GenerationConfig GenerationConfig `json:"generationConfig"`
	Tools            []Tool           `json:"tools,omitempty"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type GenerationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens"`
	Temperature     float64 `json:"temperature"`
}

type Tool struct {
	GoogleSearch *GoogleSearch `json:"googleSearch,omitempty"`
}

type GoogleSearch struct{}

// Response types

type Response struct {
	Candidates    []Candidate   `json:"candidates"`
	UsageMetadata UsageMetadata `json:"usageMetadata"`
}

type Candidate struct {
	Content       ResponseContent `json:"content"`
	FinishReason  string          `json:"finishReason"`
	SafetyRatings []SafetyRating  `json:"safetyRatings"`
}

type ResponseContent struct {
	Parts []ResponsePart `json:"parts"`
	Role  string         `json:"role"`
}

type ResponsePart struct {
	Text string `json:"text"`
}

type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type SafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

// Text returns the text from the first candidate's first part.
// Returns empty string if there are no candidates or parts.
func (r *Response) Text() string {
	if len(r.Candidates) == 0 {
		return ""
	}
	parts := r.Candidates[0].Content.Parts
	if len(parts) == 0 {
		return ""
	}
	return parts[0].Text
}
