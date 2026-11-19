package main

// 日本語向けの折り返し。glamour の WrapHook (third_party/glamour/ansi/wraphook.go) に差し込んで使う。
//
// glamour 標準の折り返しは英文前提で、空白でしか改行しない。日本語の文書では次の問題が起きる:
//   - 空白のない段落が折り返されず、端末幅を突き抜ける
//   - 「Markdown をターミナルで…」のように英単語の後の空白で改行され、日本語部分が丸ごと次行に送られる
//   - 段落内のソフト改行が空白に置き換わり、日本語の文中に不要な空白が入る
//
// ここでは日本語の組版の慣習に寄せて、次のように折り返す:
//   - 全角文字の前後ならどこでも改行してよい。英単語や数字の途中では改行しない
//   - 禁則処理: 句読点・閉じ括弧・小書きの仮名・長音符などは行頭に置かず、開き括弧は行末に置かない
//     (該当する文字は前後の文字とひとまとまりにして、まとめて次の行へ送る「追い出し」方式)
//   - 段落内のソフト改行は、前後がどちらも全角文字なら詰め、それ以外は空白 1 つにする
//   - リスト項目の 2 行目以降は、行頭記号 (•, 1., [✓] など) の幅だけ字下げして本文の左端を揃える
//   - 改行位置にある空白は捨てる
//   - 色付きの文字列を途中で改行するときは、行末で色をリセットして次の行の先頭で付け直す
//     (ページャが行単位で表示しても色が崩れないように)

import (
	"regexp"
	"strings"

	gansi "github.com/charmbracelet/glamour/ansi"
	"github.com/rivo/uniseg"
)

func init() {
	gansi.WrapHook = wrapForGlamour
}

// wrapForGlamour は glamour から呼ばれる折り返し処理の入口。
func wrapForGlamour(s string, width int, mode gansi.WrapMode) string {
	if width <= 0 {
		return strings.ReplaceAll(s, gansi.HardBreak, "\n")
	}
	switch mode {
	case gansi.WrapParagraph:
		return wrapParagraph(s, width)
	case gansi.WrapHeading:
		return wrapLines(s, width, false)
	default:
		return wrapLines(s, width, true)
	}
}

// wrapParagraph は段落本文を折り返す。ソフト改行は結合し、ハード改行だけを改行として残す。
func wrapParagraph(s string, width int) string {
	parts := strings.Split(s, gansi.HardBreak)
	out := make([]string, len(parts))
	for i, p := range parts {
		joined := joinSoftBreaks(p)
		out[i] = wrapLine(joined, width, 0, true)
	}
	return strings.Join(out, "\n")
}

// wrapLines は "\n" 区切りの各行を、幅を超えるものだけ折り返す。
// 幅に収まる行は一切変更しない (glamour の文書全体ブロックなど、すでに整形済みの行を壊さないため)。
// hanging が true なら、リストの行頭記号や行頭の空白に合わせて 2 行目以降を字下げする。
func wrapLines(s string, width int, hanging bool) string {
	s = strings.ReplaceAll(s, gansi.HardBreak, "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if uniseg.StringWidth(stripEscapes(line)) <= width {
			continue
		}
		hang := 0
		if hanging {
			hang = hangingIndent(stripEscapes(line), width)
		}
		lines[i] = wrapLine(line, width, hang, false)
	}
	return strings.Join(lines, "\n")
}

// listMarker は glamour が出力するリストの行頭 (字下げ + 記号) にマッチする。
// 箇条書き "• "、番号付き "1. "、タスク "[✓] " / "[ ] " に対応。
// ascii / notty スタイル (色なし出力) ではチェック済みタスクが "[x] " になるので、それも含める。
var listMarker = regexp.MustCompile(`^ *(?:• |\d+\. |\[[ ✓x]\] )?`)

// hangingIndent は 2 行目以降の字下げ幅を返す。
// 字下げが幅の半分以上になると本文の幅が取れないので、その場合は字下げしない。
func hangingIndent(plain string, width int) int {
	hang := uniseg.StringWidth(listMarker.FindString(plain))
	if hang*2 >= width {
		return 0
	}
	return hang
}

// ---- 字句の分解 ----

// cell は表示上の 1 文字 (書記素クラスタ)。直前にある ANSI エスケープを pre に持つ。
type cell struct {
	pre   string // この文字の前に出力するエスケープシーケンス
	text  string // 文字そのもの
	width int    // 表示幅 (0, 1, 2)
	first rune   // 先頭のコードポイント (禁則判定用)
	last  rune   // 末尾のコードポイント (禁則判定用)
}

func (c cell) isSpace() bool { return c.text == " " || c.text == "\t" }

// splitCells は文字列を cell の列と、末尾に残ったエスケープ (tail) に分解する。
func splitCells(s string) (cells []cell, tail string) {
	var pre strings.Builder
	for len(s) > 0 {
		if s[0] == 0x1b {
			n := escapeLen(s)
			pre.WriteString(s[:n])
			s = s[n:]
			continue
		}
		var cluster string
		var w int
		// 状態は毎回リセットする。エスケープを読み飛ばした後や改行の後に前の状態を引き継ぐと、
		// uniseg が次の文字を結合文字の続きとみなして幅 0 を返すことがあるため
		cluster, s, w, _ = uniseg.FirstGraphemeClusterInString(s, -1)
		rs := []rune(cluster)
		cells = append(cells, cell{
			pre:   pre.String(),
			text:  cluster,
			width: w,
			first: rs[0],
			last:  rs[len(rs)-1],
		})
		pre.Reset()
	}
	return cells, pre.String()
}

// escapeLen は s の先頭にある ESC から始まるエスケープシーケンスのバイト長を返す。
// CSI (ESC [ ... 終端文字) と OSC (ESC ] ... BEL / ESC \) を解釈し、それ以外は 2 バイトとみなす。
func escapeLen(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	switch s[1] {
	case '[':
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
		return len(s)
	case ']':
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
		return len(s)
	}
	return 2
}

// stripEscapes は ANSI エスケープを取り除いた表示文字列を返す。
func stripEscapes(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	var b strings.Builder
	for len(s) > 0 {
		if s[0] == 0x1b {
			s = s[escapeLen(s):]
			continue
		}
		i := strings.IndexByte(s, 0x1b)
		if i < 0 {
			i = len(s)
		}
		b.WriteString(s[:i])
		s = s[i:]
	}
	return b.String()
}

// ---- ソフト改行の結合 ----

// joinSoftBreaks は段落内の "\n" (ソフト改行) を結合する。
// 前後の文字がどちらも全角なら詰め、そうでなければ空白 1 つにする
// (CSS Text Level 4 の segment break の扱いと同じ考え方)。
func joinSoftBreaks(s string) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	cells, tail := splitCells(s)
	var b strings.Builder
	for i, c := range cells {
		if c.text != "\n" && c.text != "\r\n" {
			b.WriteString(c.pre)
			b.WriteString(c.text)
			continue
		}
		b.WriteString(c.pre)
		prev, next := visibleNeighbor(cells, i, -1), visibleNeighbor(cells, i, +1)
		if prev != nil && next != nil && prev.width == 2 && next.width == 2 {
			continue // 全角同士: 詰める
		}
		if prev == nil || next == nil || prev.isSpace() || next.isSpace() {
			continue // 端や空白の隣: 空白を増やさない
		}
		b.WriteString(" ")
	}
	b.WriteString(tail)
	return b.String()
}

// visibleNeighbor は cells[i] から dir 方向に見て最初の改行でない文字を返す。
func visibleNeighbor(cells []cell, i, dir int) *cell {
	for j := i + dir; j >= 0 && j < len(cells); j += dir {
		if cells[j].text != "\n" {
			return &cells[j]
		}
	}
	return nil
}

// ---- 禁則処理 ----

// 行頭禁則文字: 行の先頭に来てはいけない文字 (句読点、閉じ括弧、小書きの仮名、長音符など)。
const noStartChars = "、。，．,.:;!?)]}）］｝〕〉》」』】〙〗〟’”｠»" +
	"ゝゞヽヾー々〻‐゠–〜～？！：；・…‥" +
	"ぁぃぅぇぉっゃゅょゎゕゖァィゥェォッャュョヮヵヶㇰㇱㇲㇳㇴㇵㇶㇷㇸㇹㇺㇻㇼㇽㇾㇿ"

// 行末禁則文字: 行の末尾に来てはいけない文字 (開き括弧など)。
const noEndChars = "([{（［｛〔〈《「『【〘〖〝‘“｟«"

func isNoStart(r rune) bool { return strings.ContainsRune(noStartChars, r) }
func isNoEnd(r rune) bool   { return strings.ContainsRune(noEndChars, r) }

// canBreakBetween は、空白以外の 2 文字 a, b の間で改行してよいかを返す。
func canBreakBetween(a, b cell) bool {
	if isNoStart(b.first) || isNoEnd(a.last) {
		return false
	}
	// 全角文字が絡む位置はどこでも改行できる (日本語は単語の区切りに空白を使わないため)
	if a.width == 2 || b.width == 2 {
		return true
	}
	// それ以外 (英単語・数字・URL の途中) は改行しない。
	// ハイフンの後も改行しない: 日付 (2026-11-20)・電話番号・URL が途中で切れると読みにくいため
	// (1 行に収まらないほど長い場合だけ wrapLine が文字単位で切る)
	return false
}

// ---- 折り返し本体 ----

// unit は途中で改行しない文字のまとまり。空白は 1 文字ずつ独立した unit にする。
type unit struct {
	cells []cell
	width int
	space bool
}

// buildUnits は cell 列を、改行してよい位置で unit に区切る。
func buildUnits(cells []cell) []unit {
	var units []unit
	for i, c := range cells {
		if c.isSpace() {
			units = append(units, unit{cells: []cell{c}, width: c.width, space: true})
			continue
		}
		if len(units) > 0 {
			last := &units[len(units)-1]
			if !last.space && !canBreakBetween(cells[i-1], c) {
				last.cells = append(last.cells, c)
				last.width += c.width
				continue
			}
		}
		units = append(units, unit{cells: []cell{c}, width: c.width})
	}
	return units
}

// lineWriter は行を組み立てながら、改行位置で色の状態を引き継ぐ。
type lineWriter struct {
	b      strings.Builder
	active string // 現在有効な SGR (色・装飾) の並び。改行後に付け直す
	hang   string // 2 行目以降の字下げ
	width  int    // 現在の行の表示幅
}

// writeEscapes はエスケープを出力し、SGR の状態を追跡する。
func (w *lineWriter) writeEscapes(s string) {
	w.b.WriteString(s)
	for len(s) > 0 {
		if s[0] != 0x1b {
			s = s[1:]
			continue
		}
		n := escapeLen(s)
		seq := s[:n]
		s = s[n:]
		if !strings.HasPrefix(seq, "\x1b[") || !strings.HasSuffix(seq, "m") {
			continue
		}
		if seq == "\x1b[0m" || seq == "\x1b[m" {
			w.active = ""
		} else {
			w.active += seq
		}
	}
}

func (w *lineWriter) writeCell(c cell) {
	w.writeEscapes(c.pre)
	w.b.WriteString(c.text)
	w.width += c.width
}

// newline は改行する。色が有効なら行末でリセットし、字下げの後で付け直す
// (字下げの空白に下線や背景色が付かないように、付け直しは字下げの後)。
func (w *lineWriter) newline() {
	if w.active != "" {
		w.b.WriteString("\x1b[0m")
	}
	w.b.WriteString("\n")
	w.b.WriteString(w.hang)
	w.b.WriteString(w.active)
	w.width = len(w.hang)
}

// wrapLine は改行を含まない 1 行を width で折り返す。
// hang は 2 行目以降の字下げ幅 (1 行目の先頭 hang 桁は行頭記号としてそのまま出す)。
// trimEnd が true なら行末の空白を捨てる (段落用)。
func wrapLine(line string, width, hang int, trimEnd bool) string {
	cells, tail := splitCells(line)
	w := &lineWriter{hang: strings.Repeat(" ", hang)}

	// 行頭記号部分 (幅 hang) はそのまま出力する
	i := 0
	for i < len(cells) && w.width < hang {
		w.writeCell(cells[i])
		i++
	}

	// 改行位置で捨てる可能性がある空白。次の塊が同じ行に収まったときだけ出力する。
	// 折り返した直後は必ず塊を書くので、行頭に空白が残ることはない。
	var pending []cell
	for _, u := range buildUnits(cells[i:]) {
		if u.space {
			pending = append(pending, u.cells[0])
			continue
		}
		pw := 0
		for _, p := range pending {
			pw += p.width
		}
		if w.width+pw+u.width <= width {
			for _, p := range pending {
				w.writeCell(p)
			}
			pending = pending[:0]
			for _, c := range u.cells {
				w.writeCell(c)
			}
			continue
		}
		// 収まらない: 空白は捨てて改行する。行が空 (行頭記号のみ) なら改行しない
		for _, p := range pending {
			w.writeEscapes(p.pre)
		}
		pending = pending[:0]
		if w.width > hang {
			w.newline()
		}
		// それでも 1 行に収まらない長い塊 (長い URL や英単語) は文字単位で強制的に切る
		for _, c := range u.cells {
			if w.width+c.width > width && w.width > hang {
				w.newline()
			}
			w.writeCell(c)
		}
	}
	// 行末に残った空白。段落では捨てる。ブロックでは glamour の右側の埋め草なので、幅に収まる分だけ残す
	for _, p := range pending {
		if trimEnd || w.width+p.width > width {
			w.writeEscapes(p.pre)
		} else {
			w.writeCell(p)
		}
	}
	w.writeEscapes(tail)
	return w.b.String()
}
