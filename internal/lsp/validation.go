package lsp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"AICodeAgent/internal/agent"
)

type ValidationStatus int

const (
	ValidationStatusPassed ValidationStatus = iota
	ValidationStatusFailed
	ValidationStatusMaxIterations
	ValidationStatusRolledBack
)

type ValidationResult struct {
	FilePath         string
	Status           ValidationStatus
	Iterations       int
	CompileErrors    int
	DiagnosticsFixed int
	BestCode         string
	BestErrorCount   int
	FinalError       string
}

type ValidationLoop struct {
	coordinator   agent.Coordinator
	lspClients    *sync.Map
	maxIterations int
}

type ErrorType string

const (
	ErrorTypeSyntax    ErrorType = "syntax"
	ErrorTypeUndefined ErrorType = "undefined"
	ErrorTypeMismatch  ErrorType = "type_mismatch"
	ErrorTypeImport    ErrorType = "import"
	ErrorTypeUnused    ErrorType = "unused"
	ErrorTypeUnknown   ErrorType = "unknown"
)

type ErrorClassifier struct{}

type RepairStrategy struct {
	ErrorType      ErrorType
	Description    string
	PromptTemplate string
}

func NewValidationLoop(coordinator agent.Coordinator, clients *sync.Map, maxIterations int) *ValidationLoop {
	if maxIterations <= 0 {
		maxIterations = 5
	}
	return &ValidationLoop{coordinator: coordinator, lspClients: clients, maxIterations: maxIterations}
}

func (vl *ValidationLoop) ValidateAndRepairWithFallback(ctx context.Context, sessionID, filePath, generatedCode string) (*ValidationResult, error) {
	result := &ValidationResult{FilePath: filePath, BestCode: generatedCode, BestErrorCount: int(^uint(0) >> 1)}
	currentCode := generatedCode

	for result.Iterations < vl.maxIterations {
		result.Iterations++
		if err := os.WriteFile(filePath, []byte(currentCode), 0644); err != nil {
			result.Status = ValidationStatusFailed
			result.FinalError = err.Error()
			return result, err
		}

		errorCount, repairPrompt := vl.evaluate(ctx, filePath)
		if errorCount < result.BestErrorCount {
			result.BestErrorCount = errorCount
			result.BestCode = currentCode
		}
		if errorCount == 0 {
			result.Status = ValidationStatusPassed
			return result, nil
		}

		if result.Iterations > 1 && errorCount > result.BestErrorCount {
			_ = os.WriteFile(filePath, []byte(result.BestCode), 0644)
			result.Status = ValidationStatusRolledBack
			result.FinalError = "repair introduced more errors, rolled back to best code"
			return result, nil
		}
		if vl.coordinator == nil {
			result.Status = ValidationStatusFailed
			result.FinalError = "coordinator is nil"
			return result, errors.New(result.FinalError)
		}

		agentResult, err := vl.coordinator.Run(ctx, sessionID, repairPrompt)
		if err != nil {
			result.Status = ValidationStatusFailed
			result.FinalError = err.Error()
			return result, err
		}
		currentCode = extractCode(agentResult.Response)
	}

	_ = os.WriteFile(filePath, []byte(result.BestCode), 0644)
	result.Status = ValidationStatusMaxIterations
	result.FinalError = fmt.Sprintf("reached max iterations (%d)", vl.maxIterations)
	return result, errors.New(result.FinalError)
}

func (vl *ValidationLoop) ValidateAndRepair(ctx context.Context, sessionID, filePath, generatedCode string) (*ValidationResult, error) {
	return vl.ValidateAndRepairWithFallback(ctx, sessionID, filePath, generatedCode)
}

func (vl *ValidationLoop) evaluate(ctx context.Context, filePath string) (int, string) {
	if err := vl.compileCheck(ctx, filePath); err != nil {
		return 1, buildContextualPrompt(filePath, 1, err.Error(), readSurroundingCode(filePath))
	}
	client := vl.clientForFile(filePath)
	if client == nil {
		return 0, ""
	}
	_ = client.OpenFile(ctx, filePath)
	_ = client.NotifyChange(ctx, filePath)
	diagnostics := client.DiagnosticsForFile(filePath)
	if len(diagnostics) == 0 {
		return 0, ""
	}
	diag := diagnostics[0]
	return len(diagnostics), buildContextualPrompt(filePath, diag.Range.Start.Line+1, diag.Message, readSurroundingCode(filePath))
}

func (vl *ValidationLoop) compileCheck(ctx context.Context, filePath string) error {
	switch filepath.Ext(filePath) {
	case ".go":
		cmd := exec.CommandContext(ctx, "go", "build", filePath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("compile error: %s", strings.TrimSpace(string(output)))
		}
	case ".ts", ".tsx":
		cmd := exec.CommandContext(ctx, "tsc", "--noEmit", filePath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("compile error: %s", strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func (vl *ValidationLoop) clientForFile(filePath string) *Client {
	var matched *Client
	if vl.lspClients == nil {
		return nil
	}
	vl.lspClients.Range(func(_, value any) bool {
		client, ok := value.(*Client)
		if !ok {
			return true
		}
		if client.HandlesFile(filePath) {
			matched = client
			return false
		}
		return true
	})
	return matched
}

func (ec *ErrorClassifier) Classify(errorMsg string) ErrorType {
	msg := strings.ToLower(errorMsg)
	switch {
	case strings.Contains(msg, "syntax error") || strings.Contains(msg, "unexpected"):
		return ErrorTypeSyntax
	case strings.Contains(msg, "undefined") || strings.Contains(msg, "undeclared"):
		return ErrorTypeUndefined
	case strings.Contains(msg, "cannot use") || strings.Contains(msg, "type mismatch"):
		return ErrorTypeMismatch
	case strings.Contains(msg, "import"):
		return ErrorTypeImport
	case strings.Contains(msg, "unused") || strings.Contains(msg, "not used"):
		return ErrorTypeUnused
	default:
		return ErrorTypeUnknown
	}
}

func GetRepairStrategy(errorType ErrorType) *RepairStrategy {
	strategies := map[ErrorType]*RepairStrategy{
		ErrorTypeSyntax:    {ErrorType: ErrorTypeSyntax, Description: "修复语法错误", PromptTemplate: "请修复 {{.FilePath}} 第 {{.Line}} 行的语法错误：\n{{.ErrorMessage}}\n\n代码上下文：\n{{.SurroundingCode}}"},
		ErrorTypeUndefined: {ErrorType: ErrorTypeUndefined, Description: "修复未定义符号", PromptTemplate: "请修复 {{.FilePath}} 中的未定义符号：\n{{.ErrorMessage}}\n\n代码上下文：\n{{.SurroundingCode}}"},
		ErrorTypeMismatch:  {ErrorType: ErrorTypeMismatch, Description: "修复类型不匹配", PromptTemplate: "请修复 {{.FilePath}} 中的类型不匹配：\n{{.ErrorMessage}}\n\n代码上下文：\n{{.SurroundingCode}}"},
		ErrorTypeImport:    {ErrorType: ErrorTypeImport, Description: "修复导入问题", PromptTemplate: "请修复 {{.FilePath}} 中的 import 问题：\n{{.ErrorMessage}}\n\n代码上下文：\n{{.SurroundingCode}}"},
		ErrorTypeUnused:    {ErrorType: ErrorTypeUnused, Description: "修复未使用代码", PromptTemplate: "请修复 {{.FilePath}} 中的未使用代码：\n{{.ErrorMessage}}\n\n代码上下文：\n{{.SurroundingCode}}"},
		ErrorTypeUnknown:   {ErrorType: ErrorTypeUnknown, Description: "修复未知错误", PromptTemplate: "请修复 {{.FilePath}} 中的问题：\n{{.ErrorMessage}}\n\n代码上下文：\n{{.SurroundingCode}}"},
	}
	return strategies[errorType]
}

func buildContextualPrompt(filePath string, line int, errorMsg, surroundingCode string) string {
	classifier := &ErrorClassifier{}
	strategy := GetRepairStrategy(classifier.Classify(errorMsg))
	data := struct {
		FilePath        string
		Line            int
		ErrorMessage    string
		SurroundingCode string
	}{filePath, line, errorMsg, surroundingCode}

	tmpl, err := template.New("repair").Parse(strategy.PromptTemplate)
	if err != nil {
		return errorMsg
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return errorMsg
	}
	return sb.String()
}

func readSurroundingCode(filePath string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	return string(data)
}

func extractCode(response string) string {
	_, body, ok := strings.Cut(response, "```")
	if !ok {
		return response
	}
	if _, rest, ok := strings.Cut(body, "\n"); ok {
		body = rest
	}
	body, _, _ = strings.Cut(body, "```")
	return strings.TrimSpace(body)
}
