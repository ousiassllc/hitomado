package apptest

import (
	"context"
	"sync"

	"github.com/ousiassllc/hitomado/internal/ai"
)

// FakeClassifier は ai.Classifier（TypeSafe AI。Inbox Zero）のフェイク。工場の呼び出し（API キー）と質問の入力を記録し、
// Fn で応答を差し替える。Fn が nil なら全件「other」（確率 1）を返す。実 API には出ない。
type FakeClassifier struct {
	mu sync.Mutex

	Keys   []string           // Deps.Classifier に渡された API キー（呼び出し順）
	Inputs []ai.ClassifyInput // Classify の入力（呼び出し順。並行実行では到着順）
	Fn     func(ctx context.Context, in ai.ClassifyInput) (ai.ClassifyAnswer, error)
}

var _ ai.Classifier = (*FakeClassifier)(nil)

// Classify は入力を記録して Fn（無ければ other）で応答する。
func (f *FakeClassifier) Classify(ctx context.Context, in ai.ClassifyInput) (ai.ClassifyAnswer, error) {
	f.mu.Lock()
	f.Inputs = append(f.Inputs, in)
	fn := f.Fn
	f.mu.Unlock()
	if fn == nil {
		return ai.ClassifyAnswer{Choice: "other", Probabilities: map[string]float64{"other": 1}}, nil
	}
	return fn(ctx, in)
}

// factory は Deps.Classifier に載せる工場。API キーを記録して f を返す。
func (f *FakeClassifier) factory(apiKey string) ai.Classifier {
	f.mu.Lock()
	f.Keys = append(f.Keys, apiKey)
	f.mu.Unlock()
	return f
}

// Calls は Classify が呼ばれた回数。
func (f *FakeClassifier) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Inputs)
}

// Created は工場（Deps.Classifier）が呼ばれた回数。
func (f *FakeClassifier) Created() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Keys)
}
