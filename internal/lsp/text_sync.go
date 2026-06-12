package lsp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

type textDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int32  `json:"version"`
	Text       string `json:"text"`
}

type versionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int32  `json:"version"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument   versionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []textDocumentContentChangeEvent `json:"contentChanges"`
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type textDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

func (c *Client) OpenFile(ctx context.Context, filePath string) error {
	if !c.HandlesFile(filePath) {
		return nil
	}
	uri := uriFromPath(filePath)
	if _, ok := c.openFiles.Load(uri); ok {
		return nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	info := &OpenFileInfo{Version: 1, URI: uri, LanguageID: DetectLanguageID(filePath)}
	params := didOpenParams{TextDocument: textDocumentItem{
		URI:        uri,
		LanguageID: info.LanguageID,
		Version:    info.Version,
		Text:       string(content),
	}}
	if err := c.notify(ctx, "textDocument/didOpen", params); err != nil {
		return err
	}
	c.openFiles.Store(uri, info)
	return nil
}

func (c *Client) NotifyChange(ctx context.Context, filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return c.CloseFile(ctx, filePath)
	}
	if !c.HandlesFile(filePath) {
		return nil
	}

	uri := uriFromPath(filePath)
	value, ok := c.openFiles.Load(uri)
	if !ok {
		if err := c.OpenFile(ctx, filePath); err != nil {
			return err
		}
		value, _ = c.openFiles.Load(uri)
	}

	info := value.(*OpenFileInfo)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	version := atomic.AddInt32(&info.Version, 1)

	params := didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: version},
		ContentChanges: []textDocumentContentChangeEvent{{Text: string(content)}},
	}
	return c.notify(ctx, "textDocument/didChange", params)
}

func (c *Client) CloseFile(ctx context.Context, filePath string) error {
	uri := uriFromPath(filePath)
	if _, ok := c.openFiles.Load(uri); !ok {
		return nil
	}
	params := didCloseParams{TextDocument: textDocumentIdentifier{URI: uri}}
	if err := c.notify(ctx, "textDocument/didClose", params); err != nil {
		return err
	}
	c.openFiles.Delete(uri)
	return nil
}

func (c *Client) CloseAllFiles(ctx context.Context) error {
	var firstErr error
	c.openFiles.Range(func(key, _ any) bool {
		filePath := pathFromURI(key.(string))
		if err := c.CloseFile(ctx, filePath); err != nil && firstErr == nil {
			firstErr = err
		}
		return true
	})
	return firstErr
}

func hasFileType(filePath string, fileType string) bool {
	if fileType == "" {
		return false
	}
	if !strings.HasPrefix(fileType, ".") {
		fileType = "." + fileType
	}
	return filepath.Ext(filePath) == fileType
}
