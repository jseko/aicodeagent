package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultOpenAIBaseURL  = "https://api.openai.com/v1"
	httpClientTimeout     = 60 * time.Second
	streamChunkBufferSize = 100
	sseInitialBufferSize  = 64 * 1024
	sseMaxBufferSize      = 1024 * 1024
	errBodyReadLimit      = 4 * 1024 // 限制错误响应体读取大小
)

// OpenAIRequest OpenAI API请求体
type OpenAIRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   int64     `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

// OpenAIStreamResponse SSE行解析结构
type OpenAIStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// OpenAIProvider OpenAI提供商实现
type OpenAIProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// NewOpenAIProvider 创建OpenAI提供商实例
func NewOpenAIProvider(baseURL, apiKey, model string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = DefaultOpenAIBaseURL
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: httpClientTimeout},
	}
}

// Stream 发起流式请求，返回文本块通道
func (o *OpenAIProvider) Stream(ctx context.Context, call AgentStreamCall) (<-chan StreamingChunk, error) {
	messages := call.Messages
	if call.Prompt != "" {
		messages = append(messages, NewUserMessage(call.Prompt))
	}

	reqBody, err := json.Marshal(OpenAIRequest{
		Model:       o.model,
		Messages:    messages,
		Stream:      true,
		MaxTokens:   call.MaxTokens,
		Temperature: call.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		o.baseURL+"/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, errBodyReadLimit))
		resp.Body.Close()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan StreamingChunk, streamChunkBufferSize)
	go o.readSSE(resp.Body, ch)
	return ch, nil
}

// Chat 同步对话：内部调用Stream收集完整响应
func (o *OpenAIProvider) Chat(ctx context.Context, call AgentStreamCall) (string, error) {
	return Chat(ctx, o, call)
}

// Close 关闭资源
func (o *OpenAIProvider) Close() error {
	o.client.CloseIdleConnections()
	return nil
}

// readSSE 解析SSE流式响应
func (o *OpenAIProvider) readSSE(body io.ReadCloser, ch chan<- StreamingChunk) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	// 1MB最大缓冲区，防止超长JSON行
	scanner.Buffer(make([]byte, 0, sseInitialBufferSize), sseMaxBufferSize)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		// 结束标记
		if data == "[DONE]" {
			ch <- StreamingChunk{Done: true}
			return
		}

		var resp OpenAIStreamResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			ch <- StreamingChunk{Error: err}
			continue // 解析失败不中断流
		}

		// API返回的错误
		if resp.Error != nil {
			ch <- StreamingChunk{Error: fmt.Errorf("%s", resp.Error.Message)}
			continue
		}

		if len(resp.Choices) > 0 {
			delta := resp.Choices[0].Delta
			if delta.Content != "" {
				ch <- StreamingChunk{Content: delta.Content}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamingChunk{Error: fmt.Errorf("scanner error: %w", err)}
	}
}
