package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
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
		buf.WriteString(rewrapOverflow(out, cfg.Width))
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

// rewrapOverflow は glamour の出力のうち width を超えた行を折り返し直す。
//
// glamour (内部の reflow) は空白でしか改行しないため、空白を含まない日本語の段落は
// 1 行のまま端末幅を突き抜けてしまう。そこで描画後に、はみ出した行だけを
// ANSI エスケープを壊さずに表示幅で折り返す。折り返した続きの行には元の行の
// 字下げ (左マージンやリストのインデント) を付け直し、段落の左端を揃える。
func rewrapOverflow(rendered string, width int) string {
	if width <= 0 {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if ansi.StringWidth(line) <= width {
			out = append(out, line)
			continue
		}
		// glamour は行末を空白で埋めるので、それを除いた実体で幅を判定し直す
		trimmed := trimTrailingSpaces(line)
		if ansi.StringWidth(trimmed) <= width {
			out = append(out, trimmed)
			continue
		}
		indent := leadingSpaces(ansi.Strip(trimmed))
		if indent >= width/2 {
			// 字下げが深すぎると折り返し幅が確保できないので、字下げは諦める
			indent = 0
		}
		head := ansi.Truncate(trimmed, indent, "")
		body := ansi.TruncateLeft(trimmed, indent, "")
		wrapped := strings.Split(ansi.Wrap(body, width-indent, " "), "\n")
		pad := strings.Repeat(" ", indent)
		for i, w := range wrapped {
			if i == 0 {
				out = append(out, head+w)
			} else {
				out = append(out, pad+w)
			}
		}
	}
	return strings.Join(out, "\n")
}

// leadingSpaces は先頭の半角スペースの数を返す。
func leadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}

// trimTrailingSpaces は行末の空白を ANSI エスケープを残したまま取り除く。
// 末尾の "空白 + リセットシーケンス" のような並びにも対応するため、
// 表示上の末尾空白の数だけ表示幅を切り詰める。
func trimTrailingSpaces(line string) string {
	plain := ansi.Strip(line)
	trailing := len(plain) - len(strings.TrimRight(plain, " "))
	if trailing == 0 {
		return line
	}
	// Truncate は途中の SGR を保ったまま指定幅で切る。
	// 色付きの行なら、末尾に開きっぱなしの色が残らないようリセットを付ける (色なし出力には付けない)
	cut := ansi.Truncate(line, ansi.StringWidth(line)-trailing, "")
	if strings.Contains(line, "\x1b[") {
		cut += "\x1b[0m"
	}
	return cut
}

// fileHeader は複数ファイル表示時の区切り見出しを作る。
// 例: "── README.md ─────────"
func fileHeader(name string, cfg RenderConfig) string {
	width := cfg.Width
	if width < 20 {
		width = 20
	}
	label := "── " + name + " "
	fill := width - displayWidth(label)
	if fill < 3 {
		fill = 3
	}
	line := label + strings.Repeat("─", fill)
	if cfg.Color {
		// 太字 + 暗めの色にして本文の見出しと混同しないようにする
		return "\x1b[1;90m" + line + "\x1b[0m\n"
	}
	return line + "\n"
}

// displayWidth は見出し行の長さ計算用のおおよその表示幅。
// 東アジアの全角文字は 2 桁として数える。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if isWide(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// isWide は全角 (2 桁) で表示される代表的な文字範囲を判定する。
// 区切り線の長さ合わせにしか使わないので、厳密さより簡潔さを優先している。
func isWide(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) || // ハングル字母
		(r >= 0x2E80 && r <= 0xA4CF) || // CJK 部首〜漢字・かな
		(r >= 0xAC00 && r <= 0xD7A3) || // ハングル音節
		(r >= 0xF900 && r <= 0xFAFF) || // CJK 互換漢字
		(r >= 0xFE30 && r <= 0xFE4F) || // CJK 互換形
		(r >= 0xFF00 && r <= 0xFF60) || // 全角英数
		(r >= 0xFFE0 && r <= 0xFFE6)
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
