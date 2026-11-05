package ai

import "slices"

// エフォート（reasoning effort。#111）の段階。弱い順。
const (
	EffortLow    = "low"
	EffortMedium = "medium"
	EffortHigh   = "high"
	EffortXHigh  = "xhigh"
	EffortMax    = "max"
)

// anthropicEfforts は Anthropic が取り得る段階（弱い順）。実際に使えるのはモデルごとで、Models API の capabilities.effort で決まる。
var anthropicEfforts = []string{EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax}

// commonEfforts は OpenAI 互換と ChatGPT プランの共通の段階。一覧がモデルごとの段階を返さないため、
// 多くのモデルが受け付ける 3 段階に限る（none・minimal・xhigh などはモデルによって 400 になる）。
var commonEfforts = []string{EffortLow, EffortMedium, EffortHigh}

// EffortScale は provider が取り得るエフォートの段階（弱い順）を返す。未知のプロバイダは nil。
// モデルごとの対応（Anthropic）は ModelInfo.Efforts が持つ。
func EffortScale(provider string) []string {
	switch provider {
	case ProviderAnthropic:
		return slices.Clone(anthropicEfforts)
	case ProviderOpenAI, ProviderChatGPT:
		return slices.Clone(commonEfforts)
	}
	return nil
}

// ValidEffort は effort が provider の取り得る段階か。空（モデルの既定）は含めない。
func ValidEffort(provider, effort string) bool {
	return slices.Contains(EffortScale(provider), effort)
}
