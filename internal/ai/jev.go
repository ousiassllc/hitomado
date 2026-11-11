package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	defaultJevBaseURL = "https://api.typesafe.ai"
	jevEndpointPath   = "/v1/systemone"
	// jevModel は使うモデル（最新の Jev）。
	jevModel = "jev-latest"
	// jevQuestionID は要求内の質問の ID。応答の answers の同じ ID の下に答えが入る（モデルには見えない）。
	jevQuestionID = "category"
)

// Criterion は choice 質問の選択肢 1 つ（名前と、他と区別するための説明）。
type Criterion struct {
	Name        string
	Description string
}

// ClassifyInput は TypeSafe AI（Jev）への 1 回の choice 質問。State は判定するメールの内容、
// Criteria は選択肢（この順で要求に載せる）。
type ClassifyInput struct {
	State        string
	Instructions string
	Criteria     []Criterion
}

// ClassifyAnswer は choice 質問の答え。Choice は確率が最も高い選択肢、Probabilities は全選択肢の確率（合計 1）。
type ClassifyAnswer struct {
	Choice        string
	Probabilities map[string]float64
}

// Classifier は文章を分類する TypeSafe AI（Jev。#314）。返信下書きの Provider とは別物（文章は書かない）。
// エラーは Provider と同じ分類（ErrUnauthorized・ErrRateLimited・ErrFailed）で、キー・要求・応答の本文は含まない。
type Classifier interface {
	Classify(ctx context.Context, in ClassifyInput) (ClassifyAnswer, error)
}

// jev は Jev の System One API（POST /v1/systemone）の Classifier。
type jev struct{ client }

// NewClassifier は Jev の Classifier を返す（apiKey は TypeSafe AI の API キー）。
func NewClassifier(apiKey string) Classifier {
	c, _ := NewClassifierWithClient("", apiKey, nil) // 既定の Base URL は検証済みの定数
	return c
}

// NewClassifierWithClient は Base URL と http.Client を指定して Classifier を作る（テスト用）。
// baseURL が空なら TypeSafe AI の既定、hc が nil なら既定のクライアント。
func NewClassifierWithClient(baseURL, apiKey string, hc *http.Client) (Classifier, error) {
	base, err := normalizeBase(baseURL, defaultJevBaseURL)
	if err != nil {
		return nil, err
	}
	if hc == nil {
		hc = NewHTTPClient()
	}
	return &jev{client{hc: hc, base: base, model: jevModel, apiKey: apiKey}}, nil
}

// orderedCriteria は選択肢を、並べた順のまま JSON のオブジェクト（名前→説明）にする。
type orderedCriteria []Criterion

func (o orderedCriteria) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, c := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(c.Name)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(c.Description)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

type jevQuestion struct {
	Type         string          `json:"type"`
	Instructions string          `json:"instructions"`
	Criteria     orderedCriteria `json:"criteria"`
}

type jevRequest struct {
	State     string                 `json:"state"`
	Model     string                 `json:"model"`
	Questions map[string]jevQuestion `json:"questions"`
}

type jevResponse struct {
	Answers map[string]struct {
		Choice        string             `json:"choice"`
		Probabilities map[string]float64 `json:"probabilities"`
	} `json:"answers"`
}

// Classify は Classifier の実装。
func (j *jev) Classify(ctx context.Context, in ClassifyInput) (ClassifyAnswer, error) {
	req := jevRequest{
		State: in.State,
		Model: j.model,
		Questions: map[string]jevQuestion{
			jevQuestionID: {Type: "choice", Instructions: in.Instructions, Criteria: in.Criteria},
		},
	}
	headers := map[string]string{"Authorization": "Bearer " + j.apiKey}
	var resp jevResponse
	if err := j.postJSON(ctx, j.base+jevEndpointPath, headers, req, &resp); err != nil {
		return ClassifyAnswer{}, err
	}
	a, ok := resp.Answers[jevQuestionID]
	if !ok || a.Choice == "" {
		return ClassifyAnswer{}, fmt.Errorf("%w: 答えがありません", ErrFailed)
	}
	if _, ok := a.Probabilities[a.Choice]; !ok {
		return ClassifyAnswer{}, fmt.Errorf("%w: 確率がありません", ErrFailed)
	}
	return ClassifyAnswer{Choice: a.Choice, Probabilities: a.Probabilities}, nil
}
