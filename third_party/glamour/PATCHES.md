# glamour (vendored)

[charmbracelet/glamour](https://github.com/charmbracelet/glamour) v1.0.0 のコピー（MIT License, `LICENSE` 参照）。
mado の `go.mod` の `replace` でこちらを使う。テスト・サンプル・画像は除いてある。

## mado による変更

日本語の折り返しを mado 側で行うためのフックを追加した。`WrapHook` が nil なら上流と同じ挙動。

| ファイル | 変更 |
|---|---|
| `ansi/wraphook.go` | 新規。`WrapHook` / `WrapMode` / `HardBreak` を定義 |
| `ansi/paragraph.go` | 段落の折り返しを `WrapHook` に委譲 |
| `ansi/heading.go` | 見出しの折り返しを `WrapHook` に委譲 |
| `ansi/blockelement.go` | ブロック（リスト・引用・文書全体）の折り返しを `WrapHook` に委譲 |
| `ansi/elements.go` | `WrapHook` 設定時、ハード改行を `HardBreak` で出力してソフト改行と区別 |

上流を更新するときは、新しい版をコピーしてから上記の変更を当て直す。
