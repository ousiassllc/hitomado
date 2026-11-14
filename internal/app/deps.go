package app

import "github.com/ousiassllc/hitomado/internal/app/deps"

// Deps はサービス層が使う外部依存。実体は deps パッケージ（composecore／aicore／agentcore と循環しないよう分けた）で、(*deps.Deps)(d) で相互に変換できる。補助メソッドを持たせるため定義型にしている。
type Deps deps.Deps
