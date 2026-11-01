package main

import (
	"context"
	"fmt"
	"strings"
)

// DefaultCompactionPrompt preserves the essential instruction in Codex's
// compaction prompt: make a concise handoff that lets a later model continue
// the same task. The full upstream prompt is intentionally not duplicated.
const DefaultCompactionPrompt = `CONTEXT CHECKPOINT COMPACTION
Create a handoff summary for another LLM that will resume the task.
Capture progress, decisions, constraints, user preferences, remaining work, and critical references.`

const compactionSummaryPrefix = "Context checkpoint summary:"

// CompactionPolicy uses history-item counts rather than model tokens so the
// teaching harness can expose the lifecycle without pretending to estimate a
// production tokenizer. A zero MaxHistoryItems disables automatic compaction.
type CompactionPolicy struct {
	MaxHistoryItems       int
	RetainLatestUserItems int
}

type CompactionRequest struct {
	Prompt  string
	History []HistoryItem
}

// ContextCompactor is a separate model/service boundary. In Codex, a compact
// request is distinct from normal turn sampling and returns a handoff summary.
type ContextCompactor interface {
	Compact(context.Context, CompactionRequest) (string, error)
}

type ContextCompactorFunc func(context.Context, CompactionRequest) (string, error)

func (f ContextCompactorFunc) Compact(ctx context.Context, request CompactionRequest) (string, error) {
	return f(ctx, request)
}

func (s *Session) maybeCompact(ctx context.Context, turn *TurnContext) error {
	policy := s.compactionPolicy
	if policy.MaxHistoryItems < 1 || len(turn.history) <= policy.MaxHistoryItems {
		return nil
	}
	if s.compactor == nil {
		return fmt.Errorf("context compaction is enabled without a compactor")
	}
	request := CompactionRequest{
		Prompt:  DefaultCompactionPrompt,
		History: append([]HistoryItem(nil), turn.history...),
	}
	summary, err := s.compactor.Compact(ctx, request)
	if err != nil {
		return fmt.Errorf("compact context: %w", err)
	}
	if strings.TrimSpace(summary) == "" {
		return fmt.Errorf("compact context: empty summary")
	}
	turn.history = append(retainedUserHistory(turn.history, policy.RetainLatestUserItems), HistoryItem{
		Role:    "user",
		Content: compactionSummaryPrefix + "\n" + summary,
	})
	turn.compactionCount++
	s.emit(TurnEvent{Kind: TurnContextCompacted, Content: summary})
	return nil
}

func retainedUserHistory(history []HistoryItem, limit int) []HistoryItem {
	var users []HistoryItem
	for _, item := range history {
		if item.Role == "user" && !strings.HasPrefix(item.Content, compactionSummaryPrefix+"\n") {
			users = append(users, item)
		}
	}
	if limit > 0 && len(users) > limit {
		users = users[len(users)-limit:]
	}
	return append([]HistoryItem(nil), users...)
}
