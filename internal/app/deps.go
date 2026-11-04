package app

import "github.com/ousiassllc/hitomado/internal/app/deps"

// Deps はサービス層が使う外部依存。実体（フィールド）は deps パッケージで、サービスの内部実装パッケージ
// （composecore／aicore）と循環しないよう分けている。フィールドは同一で、(*deps.Deps)(d) で相互に変換できる。
// app パッケージ内の補助メソッド（logger／gmailErr など）を定義するため、別名ではなく定義型にしている。
type Deps deps.Deps
