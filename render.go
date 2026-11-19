package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/muesli/termenv"
	"github.com/rivo/uniseg"
)

// RenderConfig は描画方法の設定。
type RenderConfig struct {
	Style string // glamour のスタイル名
	Width int    // 折り返し幅
	Color bool   // false ならエスケープシーケンスを一切出さない
	// TrueColor は 24bit 色を使うか。false なら 256 色に落とす。
	TrueColor bool
}

// renderDocuments は全ドキュメントを描画して 1 つの文字列にまとめる。
// 複数ファイルのときは bat と同様にファイル名の見出し行で区切る。
func renderDocuments(docs []Document, cfg RenderConfig) (string, error) {
	r, err := newRenderer(cfg)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	for i, d := range docs {
		if len(docs) > 1 {
			if i > 0 {
				buf.WriteString("\n")
			}
			buf.WriteString(fileHeader(d.Name, cfg))
		}
		out, err := r.Render(string(preprocess(d.Content)))
		if err != nil {
			return "", fmt.Errorf("%s: %w", d.Name, err)
		}
		buf.WriteString(out)
	}
	return buf.String(), nil
}

func newRenderer(cfg RenderConfig) (*glamour.TermRenderer, error) {
	profile := termenv.Ascii // 色なし: SGR を出さない
	if cfg.Color {
		profile = termenv.ANSI256
		if cfg.TrueColor {
			profile = termenv.TrueColor
		}
	}
	return glamour.NewTermRenderer(
		glamour.WithStandardStyle(cfg.Style),
		glamour.WithWordWrap(cfg.Width),
		glamour.WithColorProfile(profile),
		// :tada: のような絵文字ショートコードを展開する (GitHub と同じ見え方に寄せる)
		glamour.WithEmoji(),
	)
}

// fileHeader は複数ファイル表示時の区切り見出しを作る。
// 例: "── README.md ─────────"
func fileHeader(name string, cfg RenderConfig) string {
	width := cfg.Width
	if width < 20 {
		width = 20
	}
	// 罫線 "── " と末尾の " ───" (最低 3 本) を除いた幅に名前が収まらなければ、左側を "…" で省略する。
	// 深いパスでも、区別に効くファイル名の末尾を残すため左から削る
	const minFill = 3
	name = truncateLeft(name, width-uniseg.StringWidth("── ")-1-minFill)
	label := "── " + name + " "
	fill := width - uniseg.StringWidth(label)
	line := label + strings.Repeat("─", fill)
	if cfg.Color {
		// 太字 + 暗めの色にして本文の見出しと混同しないようにする
		return "\x1b[1;90m" + line + "\x1b[0m\n"
	}
	return line + "\n"
}

// truncateLeft は s の表示幅が max を超える場合、先頭を削って "…" を付け、max 以内に収める。
func truncateLeft(s string, max int) string {
	if uniseg.StringWidth(s) <= max {
		return s
	}
	var clusters []string
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		clusters = append(clusters, g.Str())
	}
	// 末尾から詰めていき、"…" (幅 1) と合わせて max に収まるところまで残す
	w := 1
	i := len(clusters)
	for i > 0 {
		cw := uniseg.StringWidth(clusters[i-1])
		if w+cw > max {
			break
		}
		w += cw
		i--
	}
	return "…" + strings.Join(clusters[i:], "")
}

// preprocess は glamour に渡す前の Markdown を整える。
//   - 先頭の UTF-8 BOM を除去
//   - 改行コードを LF に統一
//   - YAML front matter を yaml コードブロックに変換
func preprocess(src []byte) []byte {
	b := bytesTrimBOM(src)
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	return frontMatterToCodeBlock(b)
}

// frontMatterToCodeBlock は先頭の YAML front matter (--- で囲まれた部分) を
// ```yaml コードブロックに置き換える。
// Claude Code が書く Markdown (メモリやスキル定義) は front matter 付きのことが多いが、
// そのまま渡すと "---" が水平線に、直前の行が Setext 見出しに化けて読めなくなるため。
func frontMatterToCodeBlock(b []byte) []byte {
	if !bytes.HasPrefix(b, []byte("---\n")) {
		return b
	}
	body := b[len("---\n"):]
	// 閉じの "---" は行頭・行末ちょうどのものだけを認める
	end := -1
	if bytes.HasPrefix(body, []byte("---\n")) || bytes.Equal(body, []byte("---")) {
		end = 0 // 中身が空の front matter
	} else if i := bytes.Index(body, []byte("\n---\n")); i >= 0 {
		end = i + 1
	} else if bytes.HasSuffix(body, []byte("\n---")) {
		end = len(body) - len("---")
	}
	if end < 0 {
		// 閉じがなければ front matter ではない (単なる水平線) とみなして触らない
		return b
	}
	yaml := body[:end]
	rest := body[end+len("---"):]
	rest = bytes.TrimPrefix(rest, []byte("\n"))

	var out bytes.Buffer
	out.WriteString("```yaml\n")
	out.Write(yaml)
	out.WriteString("```\n\n")
	out.Write(rest)
	return out.Bytes()
}
