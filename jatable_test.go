package main

import (
	"reflect"
	"strings"
	"testing"

	gansi "github.com/charmbracelet/glamour/ansi"
	"github.com/rivo/uniseg"
)

func TestAllocateColumns(t *testing.T) {
	tests := []struct {
		name             string
		natural, minimum []int
		avail            int
		want             []int
	}{
		// 短い列は自然な幅のまま、残りを長い列に回す
		{name: "short columns kept", natural: []int{4, 2, 80}, minimum: []int{4, 2, 2}, avail: 40, want: []int{4, 2, 34}},
		// 長い列同士は均等に分ける
		{name: "split evenly", natural: []int{50, 50}, minimum: []int{2, 2}, avail: 40, want: []int{20, 20}},
		// 端数は左の列から配る
		{name: "remainder", natural: []int{50, 50}, minimum: []int{2, 2}, avail: 41, want: []int{21, 20}},
		// 英単語などの塊の幅 (14) は下限として守る。均等割り (13) より優先
		{name: "keeps unbreakable tokens", natural: []int{14, 60, 17}, minimum: []int{14, 4, 5}, avail: 39, want: []int{14, 13, 12}},
		// 塊の合計すら入らないときは、各列に全角 1 文字分 (2 桁) を確保して均等に
		{name: "floor of two", natural: []int{30, 30, 30}, minimum: []int{20, 20, 20}, avail: 6, want: []int{2, 2, 2}},
		// 2 桁も配れないときは 1 桁
		{name: "floor of one", natural: []int{30, 30, 30}, minimum: []int{2, 2, 2}, avail: 4, want: []int{2, 1, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allocateColumns(tt.natural, tt.minimum, tt.avail)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			total := 0
			for _, w := range got {
				total += w
			}
			if total > tt.avail {
				t.Errorf("total %d exceeds %d", total, tt.avail)
			}
		})
	}
}

func TestMaxUnitWidth(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{in: "400_Frameworks", want: 14},
		{in: "Slack bot（自動）", want: 5},           // "Slack" と "（自" が最大 (開き括弧は次の字とつながる)
		{in: "あいう。", want: 4},                    // "う。" は禁則でつながる
		{in: "\x1b[1mbold\x1b[0m text", want: 4}, // エスケープは幅に数えない
	}
	for _, tt := range tests {
		if got := maxUnitWidth(tt.in); got != tt.want {
			t.Errorf("maxUnitWidth(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestWrapTableCellsLeavesFittingTable(t *testing.T) {
	rows := [][]string{{"項目", "説明"}, {"a", "短い"}}
	got := wrapTableCells(rows, 40, 10)
	if !reflect.DeepEqual(got, rows) {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestWrapTableCellsWrapsLongColumn(t *testing.T) {
	long := "句読点・閉じ括弧・小書きの仮名・長音符などは行頭に置かず、開き括弧は行末に置かない。"
	rows := [][]string{{"項目", "説明"}, {"禁則", long}}
	// セルに使える幅は 30 - 10 = 20。「項目」列は 4 桁のまま、説明列が 16 桁になる
	got := wrapTableCells(rows, 30, 10)
	if got[1][0] != "禁則" {
		t.Errorf("short column should be untouched: %q", got[1][0])
	}
	lines := strings.Split(got[1][1], "\n")
	if len(lines) < 2 {
		t.Fatalf("long cell should be wrapped: %q", lines)
	}
	for i, l := range lines {
		if w := uniseg.StringWidth(l); w > 16 {
			t.Errorf("line %d is %d wide: %q", i, w, l)
		}
		rs := []rune(l)
		if i > 0 && isNoStart(rs[0]) {
			t.Errorf("line starts with a no-start char: %q", lines)
		}
	}
	if strings.Join(lines, "") != long {
		t.Errorf("text changed: %q", lines)
	}
}

func TestWrapTableCellsConvertsBreakMarks(t *testing.T) {
	rows := [][]string{{"a"}, {"一" + gansi.SoftBreak + "二" + gansi.HardBreak + "三"}}
	got := wrapTableCells(rows, 40, 4)
	if got[1][0] != "一二\n三" {
		t.Errorf("got %q", got[1][0])
	}
}

// 実際の glamour 描画で、日本語の長いセルを含む表がどの行も幅に収まること。
func TestRenderJapaneseTableFitsWidth(t *testing.T) {
	doc := `| 項目 | 状態 | 説明 |
|---|:-:|---|
| 折り返し | 済 | 全角文字の前後ならどこでも改行してよい。英単語や数字の途中では改行しない。 |
| 禁則処理 | 済 | 句読点・閉じ括弧・小書きの仮名・長音符などは行頭に置かず、開き括弧は行末に置かない。 |
| English | ok | The quick brown fox jumps over the lazy dog. |
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
			// 幅 40 以上なら短い列 (項目・状態) は削られず、セルが 1 行のまま残る
			if width >= 40 {
				for _, cell := range []string{"折り返し", "禁則処理", "済"} {
					if !strings.Contains(stripEscapes(out), cell) {
						t.Errorf("color=%v width=%d: %q should stay whole:\n%s", color, width, cell, stripEscapes(out))
					}
				}
			}
		}
	}
}

func TestWrapTableCellsHeaderIsNeverWrapped(t *testing.T) {
	// lipgloss は見出し行を 1 行でしか表示しないので、見出しに改行を入れてはいけない
	rows := [][]string{{"A long header column one", "Another long header two"}, {"あ", "い"}}
	got := wrapTableCells(rows, 30, 8)
	for _, h := range got[0] {
		if strings.Contains(h, "\n") {
			t.Errorf("header contains a newline: %q", h)
		}
		if !strings.HasSuffix(h, "…") {
			t.Errorf("truncated header should end with an ellipsis: %q", h)
		}
	}
	total := uniseg.StringWidth(got[0][0]) + uniseg.StringWidth(got[0][1])
	if total > 30-8 {
		t.Errorf("headers are %d wide in total (> %d): %q", total, 30-8, got[0])
	}
}

func TestWrapTableCellsReservesHeaderWidth(t *testing.T) {
	// 見出しが収まる幅を確保できるなら、見出しは切り詰めずに本文の列を削る
	long := strings.Repeat("あいうえお", 10)
	rows := [][]string{{"ディレクトリ", "説明"}, {"a", long}}
	got := wrapTableCells(rows, 40, 8)
	if got[0][0] != "ディレクトリ" {
		t.Errorf("header should be kept whole: %q", got[0][0])
	}
	for _, l := range strings.Split(got[1][1], "\n") {
		if w := uniseg.StringWidth(l); w > 40-8-12 {
			t.Errorf("body line is %d wide: %q", w, l)
		}
	}
}

func TestWrapTableCellsTooNarrowLeavesItToLipgloss(t *testing.T) {
	// 各列に 2 桁も配れないときは手を出さない
	rows := [][]string{{"a", "b", "c"}, {"日本語", "日本語", "日本語"}}
	if got := wrapTableCells(rows, 10, 5); !reflect.DeepEqual(got, rows) {
		t.Errorf("got %q, want unchanged", got)
	}
}

// 表がリスト・引用の中にある場合や、列の揃え指定がある場合も、幅に収まり見出しが残ること。
func TestRenderTablesInContainersFitWidth(t *testing.T) {
	table := "| 項目 | 状態 | 説明 |\n|:--|:-:|--:|\n| 折り返し | 済 | 全角文字の前後ならどこでも改行してよい。英単語や数字の途中では改行しない。 |\n"
	docs := map[string]string{
		"plain":       table,
		"in list":     "- リスト\n\n  " + strings.ReplaceAll(strings.TrimSuffix(table, "\n"), "\n", "\n  ") + "\n",
		"in quote":    "> " + strings.ReplaceAll(strings.TrimSuffix(table, "\n"), "\n", "\n> ") + "\n",
		"header only": "| 項目 | 状態 | 説明 |\n|---|---|---|\n",
	}
	for name, doc := range docs {
		for _, style := range []string{"notty", "dark"} {
			for _, width := range []int{30, 40, 60} {
				out, err := renderDocuments([]Document{{Name: "a", Content: []byte(doc)}},
					RenderConfig{Style: style, Width: width, Color: style != "notty"})
				if err != nil {
					t.Fatal(err)
				}
				assertFits(t, out, width)
				plain := stripEscapes(out)
				for _, h := range []string{"項目", "状態", "説明"} {
					if !strings.Contains(plain, h) {
						t.Errorf("%s/%s/%d: header %q missing:\n%s", name, style, width, h, plain)
					}
				}
			}
		}
	}
}
