# glamour (vendored)

[charmbracelet/glamour](https://github.com/charmbracelet/glamour) v1.0.0 のコピー（MIT License, `LICENSE` 参照）。
mado の `go.mod` の `replace` でこちらを使う。Go のソース（テストを除く）・スタイル定義・`go.mod` / `go.sum`・`LICENSE` だけを持ってきている。テスト、テストデータ、サンプル・画像、CI 設定、ドキュメント類、スタイル生成スクリプト（`internal/generate-style-json`）は除いてある。

## mado による変更

日本語の折り返しを mado 側で行うためのフックを追加した。`WrapHook` / `TableHook` が nil なら上流と同じ挙動。

| ファイル | 変更 |
|---|---|
| `ansi/wraphook.go` | 新規。`WrapHook` / `TableHook` / `WrapMode` / `HardBreak` / `SoftBreak` を定義 |
| `ansi/paragraph.go` | 段落の折り返しを `WrapHook` に委譲 |
| `ansi/heading.go` | 見出しの折り返しを `WrapHook` に委譲 |
| `ansi/blockelement.go` | ブロック（リスト・引用・文書全体）の折り返しを `WrapHook` に委譲 |
| `ansi/elements.go` | `WrapHook` 設定時、ハード改行を `HardBreak`、ソフト改行を `SoftBreak` で出力する（リストのブロック内で、項目内の改行と項目同士の区切りを区別するため） |
| `ansi/table.go` | `TableHook` 設定時、全行を溜めておき、罫線・余白の幅を測ったうえで `TableHook` で整形してから lipgloss に渡す |

上流を更新するときは、新しい版をコピーしてから上記の変更を当て直す。
