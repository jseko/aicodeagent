package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	Tools       []any     `json:"tools,omitempty"`
	ToolChoice  string    `json:"tool_choice,omitempty"`
	MaxTokens   int64     `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

// OpenAIStreamResponse SSE行解析结构
type OpenAIStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content          string           `json:"content"`
			ReasoningContent string           `json:"reasoning_content"`
			ToolCalls        []toolCallDelta  `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// toolCallDelta OpenAI流式tool_calls增量（内部解析用）
type toolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
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
		Tools:       call.Tools,
		ToolChoice:  "auto",
		MaxTokens:   call.MaxTokens,
		Temperature: call.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	log.Printf("[OpenAI] 请求模型=%s 消息数=%d 工具数=%d 请求体=%d字节",
		o.model, len(messages), len(call.Tools), len(reqBody))

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
	go o.readSSE(ctx, resp.Body, ch)
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

// readSSE 解析SSE流式响应，累积tool_calls delta并在流结束时统一发送
func (o *OpenAIProvider) readSSE(ctx context.Context, body io.ReadCloser, ch chan<- StreamingChunk) {
	defer body.Close()
	defer close(ch)
	defer func() {
		if r := recover(); r != nil {
			ch <- StreamingChunk{Error: fmt.Errorf("readSSE panic: %v", r)}
		}
	}()

	// 监听context取消，主动关闭body以解除scanner阻塞
	go func() {
		<-ctx.Done()
		body.Close()
	}()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, sseInitialBufferSize), sseMaxBufferSize)

	log.Printf("[OpenAI] 开始读取SSE流")

	// 累积tool_calls：key=index
	toolCallsAcc := make(map[int]*ToolCallDelta)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			log.Printf("[OpenAI] SSE流完成，工具调用=%d个", len(toolCallsAcc))
			// 发送累积的tool_calls
			if len(toolCallsAcc) > 0 {
				toolCalls := make([]ToolCallDelta, 0, len(toolCallsAcc))
				for _, tc := range toolCallsAcc {
					toolCalls = append(toolCalls, *tc)
				}
				ch <- StreamingChunk{ToolCalls: toolCalls}
			}
			ch <- StreamingChunk{Done: true}
			return
		}

		var resp OpenAIStreamResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			ch <- StreamingChunk{Error: err}
			continue
		}

		if resp.Error != nil {
			ch <- StreamingChunk{Error: fmt.Errorf("%s", resp.Error.Message)}
			continue
		}

		if len(resp.Choices) > 0 {
			delta := resp.Choices[0].Delta

			// 累积tool_calls delta
			for _, tc := range delta.ToolCalls {
				existing, ok := toolCallsAcc[tc.Index]
				if !ok {
					existing = &ToolCallDelta{ID: tc.ID, Type: tc.Type}
					existing.Function.Name = tc.Function.Name
					toolCallsAcc[tc.Index] = existing
				}
				if tc.Function.Arguments != "" {
					existing.Function.Arguments += tc.Function.Arguments
				}
			}

			// 将思考内容作为文本输出（DeepSeek-R1等推理模型）
			if delta.ReasoningContent != "" {
				ch <- StreamingChunk{Content: delta.ReasoningContent}
			}
			if delta.Content != "" {
				ch <- StreamingChunk{Content: delta.Content}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamingChunk{Error: fmt.Errorf("scanner error: %w", err)}
	}
	// 流异常结束（未收到[DONE]），发送完成信号避免调用方永久阻塞
	log.Printf("[OpenAI] SSE流异常结束（未收到[DONE]），发送合成完成信号")
	ch <- StreamingChunk{Done: true}
}
