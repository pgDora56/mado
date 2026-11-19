package main

import (
	"strings"
	"testing"

	gansi "github.com/pgDora56/mado/third_party/glamour/ansi"
	"github.com/rivo/uniseg"
)

// plainLines は折り返し結果をエスケープなしの行に分ける。
func plainLines(s string) []string {
	return strings.Split(stripEscapes(s), "\n")
}

func assertFits(t *testing.T, out string, width int) {
	t.Helper()
	for i, l := range plainLines(out) {
		if w := uniseg.StringWidth(l); w > width {
			t.Errorf("line %d is %d wide (> %d): %q", i, w, width, l)
		}
	}
}

func TestWrapParagraphBreaksJapaneseAnywhere(t *testing.T) {
	in := strings.Repeat("あいうえおかきくけこ", 5) // 空白なし 100 桁
	out := wrapParagraph(in, 20)
	lines := plainLines(out)
	if len(lines) != 5 {
		t.Fatalf("want 5 lines, got %d: %q", len(lines), lines)
	}
	if strings.Join(lines, "") != in {
		t.Errorf("text changed: %q", lines)
	}
}

func TestWrapParagraphDoesNotPushJapaneseAfterEnglishWord(t *testing.T) {
	// glamour 標準だと "Markdown" の後の空白で改行され、日本語部分が丸ごと次行に送られていた
	out := wrapParagraph("Markdown をターミナルでサッと確認するための閲覧専用ビューア。", 30)
	lines := plainLines(out)
	if lines[0] == "Markdown" {
		t.Errorf("Japanese text should fill the first line: %q", lines)
	}
	assertFits(t, out, 30)
}

func TestWrapParagraphKeepsEnglishWordsWhole(t *testing.T) {
	out := wrapParagraph("hello wonderful world of terminals", 12)
	for _, l := range plainLines(out) {
		for _, word := range strings.Fields(l) {
			if !strings.Contains("hello wonderful world of terminals", word) {
				t.Errorf("word split: %q", l)
			}
		}
		if strings.HasPrefix(l, " ") || strings.HasSuffix(l, " ") {
			t.Errorf("spaces at a line edge: %q", l)
		}
	}
}

func TestWrapParagraphKeepsHyphenatedNumbers(t *testing.T) {
	// 日付・電話番号・ハイフン入りの語はハイフンの後で切らない
	for _, token := range []string{"2026-11-20", "090-1234-5678", "foo-bar"} {
		out := wrapParagraph("あいうえおかきく"+token+"です", 20)
		if !strings.Contains(stripEscapes(out), token) {
			t.Errorf("%s was split: %q", token, plainLines(out))
		}
	}
}

func TestWrapParagraphKinsoku(t *testing.T) {
	tests := []struct {
		name string
		in   string
		// 各行の先頭/末尾に来てはいけない文字
	}{
		{name: "句点", in: "あいうえおかきくけ。こさしすせそ"},
		{name: "読点", in: "あいうえおかきくけ、こさしすせそ"},
		{name: "閉じ括弧", in: "あいうえおかきく」けこさしすせそ"},
		{name: "小書き仮名", in: "あいうえおかきくけっこさしすせそ"},
		{name: "長音符", in: "あいうえおかきくけーこさしすせそ"},
		{name: "開き括弧", in: "あいうえおかきく「けこさしすせそ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 幅 18 = 全角 9 文字。10 文字目が禁則文字になるよう入力を組んである
			out := wrapParagraph(tt.in, 18)
			lines := plainLines(out)
			for i, l := range lines {
				rs := []rune(l)
				if i > 0 && isNoStart(rs[0]) {
					t.Errorf("line starts with a no-start char: %q", lines)
				}
				if i < len(lines)-1 && isNoEnd(rs[len(rs)-1]) {
					t.Errorf("line ends with a no-end char: %q", lines)
				}
			}
			if strings.Join(lines, "") != tt.in {
				t.Errorf("text changed: %q", lines)
			}
			assertFits(t, out, 18)
		})
	}
}

func TestWrapParagraphSoftAndHardBreaks(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{name: "全角同士は詰める", in: "日本語の\n段落", want: "日本語の段落"},
		{name: "英語は空白", in: "hello\nworld", want: "hello world"},
		{name: "全角と英語の間は空白", in: "日本語\nEnglish", want: "日本語 English"},
		{name: "空白の隣は空白を増やさない", in: "hello \nworld", want: "hello world"},
		{name: "ハード改行は残す", in: "一行目" + gansi.HardBreak + "二行目", want: "一行目\n二行目"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripEscapes(wrapParagraph(tt.in, 80)); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWrapParagraphSoftBreakAcrossEscapes(t *testing.T) {
	// glamour は単語ごとに色を付けるので、改行の前後にエスケープが挟まっていても判定できること
	in := "\x1b[1m日本語\x1b[0m\n\x1b[1m段落\x1b[0m"
	if got := stripEscapes(wrapParagraph(in, 80)); got != "日本語段落" {
		t.Errorf("got %q", got)
	}
}

func TestWrapParagraphHardSplitsLongURL(t *testing.T) {
	url := "https://example.com/" + strings.Repeat("a", 50)
	out := wrapParagraph("参照: "+url, 30)
	assertFits(t, out, 30)
	joined := strings.Join(plainLines(out), "")
	if !strings.Contains(joined, url) {
		t.Errorf("URL lost: %q", joined)
	}
}

func TestWrapLinesHangingIndentForLists(t *testing.T) {
	tests := []struct {
		name, line, indent string
	}{
		{name: "bullet", line: "• " + strings.Repeat("あ", 30), indent: "  "},
		{name: "nested bullet", line: "    • " + strings.Repeat("あ", 30), indent: "      "},
		{name: "ordered", line: "12. " + strings.Repeat("あ", 30), indent: "    "},
		{name: "task", line: "[✓] " + strings.Repeat("あ", 30), indent: "    "},
		{name: "task in ascii style", line: "[x] " + strings.Repeat("あ", 30), indent: "    "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := wrapLines(tt.line, 30, true)
			lines := plainLines(out)
			if len(lines) < 2 {
				t.Fatalf("expected wrapping: %q", lines)
			}
			for _, l := range lines[1:] {
				if !strings.HasPrefix(l, tt.indent) || strings.HasPrefix(l, tt.indent+" ") {
					t.Errorf("continuation should be indented by %d: %q", len(tt.indent), l)
				}
			}
			assertFits(t, out, 30)
		})
	}
}

func TestWrapLinesLeavesFittingLinesUntouched(t *testing.T) {
	// glamour の文書全体ブロックには整形済みの行 (右端の埋め草つき) が来る。幅内なら一切触らない
	in := "\x1b[38;5;252m  短い行\x1b[0m   \n  もう一行"
	if got := wrapLines(in, 20, true); got != in {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestWrapRestoresColorAcrossBreaks(t *testing.T) {
	// 色付きの文字列を途中で改行したら、行末でリセットし次の行で付け直す
	in := "\x1b[1;31m" + strings.Repeat("あ", 20) + "\x1b[0m"
	out := wrapParagraph(in, 20)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %q", lines)
	}
	if !strings.HasSuffix(lines[0], "\x1b[0m") {
		t.Errorf("first line should end with a reset: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "\x1b[1;31m") {
		t.Errorf("second line should restore the color: %q", lines[1])
	}
}

func TestWrapIndentIsNotColored(t *testing.T) {
	// 字下げの空白に色 (下線や背景) が乗らないよう、色の付け直しは字下げの後
	in := "• \x1b[4m" + strings.Repeat("あ", 20) + "\x1b[0m"
	out := wrapLines(in, 20, true)
	lines := strings.Split(out, "\n")
	if !strings.HasPrefix(lines[1], "  \x1b[4m") {
		t.Errorf("color should be restored after the indent: %q", lines[1])
	}
}

func TestStripEscapes(t *testing.T) {
	in := "\x1b[1mbold\x1b[0m \x1b]8;;https://x\x1b\\link\x1b]8;;\x07"
	if got := stripEscapes(in); got != "bold link" {
		t.Errorf("got %q", got)
	}
}

// 実際の glamour 描画を通して、日本語の文書がどの行も幅に収まることを確認する。
func TestRenderJapaneseDocumentFitsWidth(t *testing.T) {
	doc := `# 日本語の折り返しテスト：見出しが長い場合でもきちんと折り返されるかを確認する

Markdown をターミナルでサッと確認するための閲覧専用ビューア。Claude Code が書き出した計画書・レポート・メモを、エディタを開かずに読む用途を想定している。

- リスト項目も長い場合には折り返されて、二行目以降は本文の左端に揃って表示されるはずです。
  - ネストしたリストの項目も同様に、行頭記号の幅だけ字下げして折り返されるはずです。

> 引用の中の長い日本語の文章も、引用記号を保ったまま折り返されることを確認するための文章です。
`
	for _, color := range []bool{false, true} {
		style := "notty"
		if color {
			style = "dark"
		}
		for _, width := range []int{30, 40, 60} {
			out, err := renderDocuments([]Document{{Name: "a", Content: []byte(doc)}},
				RenderConfig{Style: style, Width: width, Color: color})
			if err != nil {
				t.Fatal(err)
			}
			assertFits(t, out, width)
			flat := strings.NewReplacer("\n", "", " ", "").Replace(stripEscapes(out))
			if !strings.Contains(flat, "閲覧専用ビューア") {
				t.Errorf("color=%v width=%d: text missing", color, width)
			}
		}
	}
}

func TestWrapParagraphSoftBreakMark(t *testing.T) {
	// glamour は WrapHook 設定時、ソフト改行を "\n" ではなく SoftBreak の目印で渡してくる
	tests := []struct{ in, want string }{
		{in: "日本語の" + gansi.SoftBreak + "段落", want: "日本語の段落"},
		{in: "hello" + gansi.SoftBreak + "world", want: "hello world"},
	}
	for _, tt := range tests {
		if got := stripEscapes(wrapParagraph(tt.in, 80)); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWrapParagraphHardBreakDropsSurroundingSpaces(t *testing.T) {
	// 改行の前後の空白は行末・行頭に残さない
	got := stripEscapes(wrapParagraph("一行目 "+gansi.HardBreak+" 二行目", 80))
	if got != "一行目\n二行目" {
		t.Errorf("got %q", got)
	}
}

func TestWrapLinesListItemSoftBreak(t *testing.T) {
	// リストのブロックでは "\n" は項目同士の区切りなので残し、項目内の目印だけ結合する
	in := "• 一行目の文章" + gansi.SoftBreak + "二行目\n• English" + gansi.SoftBreak + "line"
	got := stripEscapes(wrapLines(in, 80, true))
	if want := "• 一行目の文章二行目\n• English line"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWrapLinesListItemHardBreakIsIndented(t *testing.T) {
	// 幅に収まる行でも、項目内のハード改行は行頭記号の幅だけ字下げして改行する
	tests := []struct{ in, want string }{
		{in: "• 一行目" + gansi.HardBreak + "二行目", want: "• 一行目\n  二行目"},
		{in: "    • 入れ子" + gansi.HardBreak + "続き", want: "    • 入れ子\n      続き"},
		{in: "10. 番号" + gansi.HardBreak + "続き", want: "10. 番号\n    続き"},
	}
	for _, tt := range tests {
		if got := stripEscapes(wrapLines(tt.in, 40, true)); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

func TestWrapLinesHardBreakThenWrapKeepsIndent(t *testing.T) {
	// ハード改行の後の行が幅を超えても、折り返しの字下げはそろったまま
	in := "• 短い" + gansi.HardBreak + strings.Repeat("あ", 30)
	out := wrapLines(in, 20, true)
	lines := plainLines(out)
	if len(lines) < 3 {
		t.Fatalf("expected wrapping: %q", lines)
	}
	for _, l := range lines[1:] {
		if !strings.HasPrefix(l, "  あ") {
			t.Errorf("continuation should be indented by 2: %q", lines)
		}
	}
	assertFits(t, out, 20)
}

func TestWrapRestoresColorAcrossHardBreak(t *testing.T) {
	in := "• \x1b[1m太字" + gansi.HardBreak + "続き\x1b[0m"
	lines := strings.Split(wrapLines(in, 40, true), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %q", lines)
	}
	if !strings.HasSuffix(lines[0], "\x1b[0m") || !strings.HasPrefix(lines[1], "  \x1b[1m") {
		t.Errorf("color should be reset and restored after the indent: %q", lines)
	}
}

func TestWrapForGlamourWithoutWidthRemovesMarks(t *testing.T) {
	// 折り返し幅が無い場合も、目印は必ず消す
	in := "• 一" + gansi.SoftBreak + "二" + gansi.HardBreak + "三\n• 四"
	got := wrapForGlamour(in, 0, gansi.WrapBlock)
	if want := "• 一二\n三\n• 四"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// 実際の glamour 描画で、リスト項目内のソフト改行が結合され、ハード改行が字下げされること。
func TestRenderListItemBreaks(t *testing.T) {
	doc := "- 一行目の文章\n  二行目は詰める\n- ハード改行の行  \n  ここは字下げされる\n- English line\n  continues\n"
	out, err := renderDocuments([]Document{{Name: "a", Content: []byte(doc)}},
		RenderConfig{Style: "notty", Width: 60, Color: false})
	if err != nil {
		t.Fatal(err)
	}
	plain := stripEscapes(out)
	for _, want := range []string{"• 一行目の文章二行目は詰める", "• ハード改行の行", "• English line continues"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q in:\n%s", want, plain)
		}
	}
	// ハード改行の次の行は、行頭記号 "• " の後ろ (本文の左端) から始まる
	lines := strings.Split(plain, "\n")
	for i, l := range lines {
		if !strings.Contains(l, "• ハード改行の行") || i+1 >= len(lines) {
			continue
		}
		bodyCol := uniseg.StringWidth(l[:strings.Index(l, "ハード")])
		next := lines[i+1]
		if !strings.HasPrefix(strings.TrimLeft(next, " "), "ここは字下げされる") {
			t.Errorf("unexpected continuation: %q", next)
		}
		if got := len(next) - len(strings.TrimLeft(next, " ")); got != bodyCol {
			t.Errorf("continuation indent = %d, want %d:\n%s", got, bodyCol, plain)
		}
	}
	if strings.ContainsAny(out, gansi.HardBreak+gansi.SoftBreak) {
		t.Errorf("break marks leaked: %q", out)
	}
}
