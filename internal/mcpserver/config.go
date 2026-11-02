package mcpserver

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// 既定の待ち受け先。
const (
	DefaultHost = "127.0.0.1"
	DefaultPort = 47650
)

// Config は MCP サーバーの設定。既定（ゼロ値）はオフ。
type Config struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	// AllowNonLoopback は loopback 以外のアドレス（tailnet など）で待ち受けることを明示的に許可する。
	AllowNonLoopback bool `json:"allowNonLoopback"`
}

// Normalize は空の項目を既定で埋める。
func (c Config) Normalize() Config {
	c.Host = strings.TrimSpace(c.Host)
	if c.Host == "" {
		c.Host = DefaultHost
	}
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	return c
}

// IsLoopbackHost は host が loopback（localhost・127.0.0.0/8・::1）かを返す。
func IsLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// Validate は設定が使えるかを確かめる。loopback 以外は AllowNonLoopback が必要。
func (c Config) Validate() error {
	if c.Port < 1024 || c.Port > 65535 {
		return errors.New("ポートは 1024〜65535 で指定してください。")
	}
	if strings.ContainsAny(c.Host, " /\\?#@") || c.Host == "" {
		return errors.New("待ち受けアドレスが正しくありません。")
	}
	if !IsLoopbackHost(c.Host) && !c.AllowNonLoopback {
		return errors.New("loopback 以外のアドレスで待ち受けるには、明示的な許可が必要です。")
	}
	return nil
}

// Addr は net.Listen に渡す host:port。
func (c Config) Addr() string { return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) }

// EndpointURL は MCP クライアントに設定する URL。
func (c Config) EndpointURL() string { return fmt.Sprintf("http://%s/mcp", c.Addr()) }
