package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
)

// JSONRPCMessage JSON-RPC 2.0 基础消息结构
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError JSON-RPC 错误结构
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ProtocolHandler MCP 协议处理器，管理 JSON-RPC 请求-响应匹配
type ProtocolHandler struct {
	transport Transport
	nextID    atomic.Int32
	pending   sync.Map // map[int]chan *JSONRPCMessage
	onNotify  func(method string, params json.RawMessage)
}

// NewProtocolHandler 创建协议处理器实例
func NewProtocolHandler(transport Transport) *ProtocolHandler {
	return &ProtocolHandler{transport: transport}
}

// SetNotifyHandler 设置通知处理回调
func (p *ProtocolHandler) SetNotifyHandler(handler func(method string, params json.RawMessage)) {
	p.onNotify = handler
}

// StartReceiveLoop 启动后台消息接收循环
func (p *ProtocolHandler) StartReceiveLoop(ctx context.Context) {
	go p.receiveLoop(ctx)
}

// Call 发送 JSON-RPC 请求并等待响应
func (p *ProtocolHandler) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := int(p.nextID.Add(1))
	respCh := make(chan *JSONRPCMessage, 1)
	p.pending.Store(id, respCh)
	defer p.pending.Delete(id)

	// 序列化参数
	var rawParams json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
		rawParams = data
	}

	// 构造并发送请求
	req := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  rawParams,
	}
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	if err := p.transport.Send(ctx, reqData); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// 等待响应（带超时）
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

// SendNotification 发送 JSON-RPC 通知（无需响应）
func (p *ProtocolHandler) SendNotification(ctx context.Context, method string, params any) error {
	var rawParams json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params: %w", err)
		}
		rawParams = data
	}

	req := JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  rawParams,
	}
	reqData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	return p.transport.Send(ctx, reqData)
}

// receiveLoop 后台消息接收循环
func (p *ProtocolHandler) receiveLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data, err := p.transport.Receive(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("[MCP Protocol] 接收消息失败: %v", err)
				continue
			}
		}

		// 去除末尾空白
		data = dropCR(bytesTrimRight(data))

		var msg JSONRPCMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("[MCP Protocol] 消息解析失败: %v, data=%s", err, string(data))
			continue
		}

		if id, ok := responseID(data); ok {
			if ch, ok := p.pending.Load(id); ok {
				ch.(chan *JSONRPCMessage) <- &msg
			} else if p.onNotify != nil {
				p.onNotify(msg.Method, msg.Params)
			} else {
				log.Printf("[MCP Protocol] 未找到对应请求 ID=%d", id)
			}
		} else if p.onNotify != nil {
			p.onNotify(msg.Method, msg.Params)
		}
	}
}

func bytesTrimRight(data []byte) []byte {
	for len(data) > 0 && (data[len(data)-1] == '\n' || data[len(data)-1] == '\r' || data[len(data)-1] == ' ') {
		data = data[:len(data)-1]
	}
	return data
}

// responseID 从 JSON-RPC 原始消息中解析 ID，兼容 LSP 的字符串 ID 和 MCP 的整数 ID。
// 本方法不依赖 JSONRPCMessage.ID 字段，可接受服务端返回的任意类型 ID。

func responseID(data []byte) (int, bool) {
	var raw struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0, false
	}
	if len(raw.ID) == 0 {
		return 0, false
	}
	var intID int
	if err := json.Unmarshal(raw.ID, &intID); err == nil {
		return intID, true
	}
	var strID string
	if err := json.Unmarshal(raw.ID, &strID); err == nil {
		if n, err := fmt.Sscanf(strID, "%d", &intID); err == nil && n == 1 {
			return intID, true
		}
	}
	return 0, false
}
