//go:build tools

// 実装中に依存を go.sum へ固定するための一時ファイル（実装完了後に削除し go mod tidy する）。
package main

import (
	_ "github.com/hashicorp/golang-lru/v2"
	_ "github.com/jhillyerd/enmime/v2"
	_ "github.com/oklog/ulid/v2"
	_ "github.com/pressly/goose/v3"
	_ "github.com/stretchr/testify/assert"
	_ "github.com/stretchr/testify/require"
	_ "github.com/zalando/go-keyring"
	_ "golang.org/x/net/html"
	_ "golang.org/x/oauth2"
	_ "golang.org/x/oauth2/google"
	_ "golang.org/x/time/rate"
	_ "google.golang.org/api/gmail/v1"
	_ "google.golang.org/api/option"
	_ "modernc.org/sqlite"
)
