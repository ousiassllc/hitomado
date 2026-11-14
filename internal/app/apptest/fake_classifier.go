package apptest

import (
	"context"
	"sync"

	"github.com/ousiassllc/hitomado/internal/ai"
)

// FakeClassifier は ai.Classifier（Inbox Zero の判定）のフェイク。工場の呼び出し（選んだ AI・API キー）と質問の入力を記録し、
// Fn で応答を差し替える。Fn が nil なら全件「other」（確率 1）を返す。実 API には出ない。
type FakeClassifier struct {
	mu sync.Mutex

	Keys   []string           // Deps.Classifier に渡された API キー（呼び出し順）
	Made   []ClassifierMade   // Deps.Classifier に渡された、選んだ AI（呼び出し順。キーは Keys と同じ並び）
	Inputs []ai.ClassifyInput // Classify の入力（呼び出し順。並行実行では到着順）
	Fn     func(ctx context.Context, in ai.ClassifyInput) (ai.ClassifyAnswer, error)
}

// ClassifierMade は Deps.Classifier（工場）に渡された、選んだ AI（API キーは Keys に記録する）。
type ClassifierMade struct{ Provider, Model, BaseURL string }

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

// factory は Deps.Classifier に載せる工場。選んだ AI と API キーを記録して f を返す。
func (f *FakeClassifier) factory(provider, model, baseURL, apiKey string) (ai.Classifier, error) {
	f.mu.Lock()
	f.Keys = append(f.Keys, apiKey)
	f.Made = append(f.Made, ClassifierMade{Provider: provider, Model: model, BaseURL: baseURL})
	f.mu.Unlock()
	return f, nil
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
