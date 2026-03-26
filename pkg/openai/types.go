package openai

type ChatCompletionRequest struct {
	Model               string             `json:"model"`
	Messages            []ChatMessage      `json:"messages"`
	Temperature         *float64           `json:"temperature,omitempty"`
	TopP                *float64           `json:"top_p,omitempty"`
	N                   *int               `json:"n,omitempty"`
	Stream              bool               `json:"stream,omitempty"`
	StreamOptions       *StreamOptions     `json:"stream_options,omitempty"`
	Stop                interface{}        `json:"stop,omitempty"`
	MaxTokens           *int               `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int               `json:"max_completion_tokens,omitempty"`
	PresencePenalty     *float64           `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64           `json:"frequency_penalty,omitempty"`
	LogitBias           map[string]float64 `json:"logit_bias,omitempty"`
	Logprobs            *bool              `json:"logprobs,omitempty"`
	TopLogprobs         *int               `json:"top_logprobs,omitempty"`
	User                string             `json:"user,omitempty"`
	Tools               []Tool             `json:"tools,omitempty"`
	ToolChoice          interface{}        `json:"tool_choice,omitempty"`
	ResponseFormat      *ResponseFormat    `json:"response_format,omitempty"`
	Seed                *int64             `json:"seed,omitempty"`
}

type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type ResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema interface{} `json:"json_schema,omitempty"`
}

type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   *Usage                 `json:"usage,omitempty"`
}

type ChatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
	Logprobs     interface{} `json:"logprobs,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param,omitempty"`
	Code    string `json:"code,omitempty"`
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelList struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}
