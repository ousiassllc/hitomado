//go:build !android

package platform

import "fmt"

// Open は URL をシステムブラウザ／メーラーで開く。
func (b *Browser) Open(url string) error {
	if err := b.app.Browser.OpenURL(url); err != nil {
		return fmt.Errorf("open url: %w", err)
	}
	return nil
}

// OpenPath はパスを OS の標準の方法で開く。
func (p *Paths) OpenPath(path string) error {
	if err := p.app.Browser.OpenFile(path); err != nil {
		return fmt.Errorf("open path: %w", err)
	}
	return nil
}
