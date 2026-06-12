package subagent

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type HotReloadEventType string

const (
	HotReloadCreated HotReloadEventType = "created"
	HotReloadUpdated HotReloadEventType = "updated"
	HotReloadRemoved HotReloadEventType = "removed"
)

type HotReloadEvent struct {
	Type HotReloadEventType
	Path string
	Time time.Time
}

type HotReloadableLoader struct {
	Loader   *SubagentLoader
	Interval time.Duration
	Events   chan HotReloadEvent
	Logger   *slog.Logger
}

func NewHotReloadableLoader(loader *SubagentLoader, interval time.Duration) *HotReloadableLoader {
	if interval <= 0 {
		interval = time.Second
	}
	return &HotReloadableLoader{Loader: loader, Interval: interval, Events: make(chan HotReloadEvent, 16)}
}

func (h *HotReloadableLoader) WatchHotReload(ctx context.Context) error {
	if h == nil || h.Loader == nil {
		return nil
	}
	known := make(map[string]time.Time)
	ticker := time.NewTicker(h.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			current := h.snapshot()
			h.emitDiff(known, current)
			known = current
		}
	}
}

func (h *HotReloadableLoader) snapshot() map[string]time.Time {
	result := make(map[string]time.Time)
	for _, dir := range []string{h.Loader.UserDir, h.Loader.ProjectDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			info, err := entry.Info()
			if err != nil {
				continue
			}
			result[path] = info.ModTime()
		}
	}
	return result
}

func (h *HotReloadableLoader) emitDiff(old, current map[string]time.Time) {
	for path, mod := range current {
		oldMod, exists := old[path]
		if !exists {
			h.reload(path, HotReloadCreated)
			continue
		}
		if mod.After(oldMod) {
			h.reload(path, HotReloadUpdated)
		}
	}
	for path := range old {
		if _, exists := current[path]; !exists {
			h.unload(path)
		}
	}
}

func (h *HotReloadableLoader) reload(path string, eventType HotReloadEventType) {
	_ = h.Loader.loadOne(path, true)
	h.publish(HotReloadEvent{Type: eventType, Path: path, Time: time.Now()})
}

func (h *HotReloadableLoader) unload(path string) {
	h.publish(HotReloadEvent{Type: HotReloadRemoved, Path: path, Time: time.Now()})
}

func (h *HotReloadableLoader) publish(event HotReloadEvent) {
	select {
	case h.Events <- event:
	default:
	}
}
