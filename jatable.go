package main

// 表のセルの日本語向け折り返し。glamour の TableHook (third_party/glamour/ansi/wraphook.go) に差し込んで使う。
//
// glamour は表の描画を lipgloss に任せており、lipgloss は列幅が足りないとセルを空白の位置でしか
// 折り返さない (禁則処理もない)。日本語のセルは空白がないので、文字単位で切られて禁則も崩れる。
// そこで lipgloss に渡す前に列幅を自前で決め、各セルを wrapLine で折り返しておく。
// 折り返し済みのセルが列幅に収まっていれば、lipgloss は列を広げるだけで再度折り返さない。
//
// ただし lipgloss は見出し行の高さを常に 1 にする (折り返すと 2 行目以降が黙って消える)。
// そのため見出しは折り返さず、なるべく見出しが収まる列幅を確保し、無理なら末尾を "…" で切り詰める。

import (
	"sort"
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
	gansi "github.com/pgDora56/mado/third_party/glamour/ansi"
	"github.com/rivo/uniseg"
)

func init() {
	gansi.TableHook = wrapTableCells
}

// wrapTableCells は表の全セル (1 行目はヘッダ) を、表全体が width に収まるように折り返す。
// overhead は罫線と余白が占める幅で、セルに使えるのは width - overhead。
func wrapTableCells(rows [][]string, width, overhead int) [][]string {
	cols := 0
	for _, row := range rows {
		cols = max(cols, len(row))
	}

	// セルの改行の目印を整え、各列の自然な幅 (折り返さない場合の最大幅)・
	// 最小幅 (途中で切れない塊のうち最大のもの。英単語・数字・禁則でつながった文字列)・
	// 見出しの幅を測る
	out := make([][]string, len(rows))
	natural := make([]int, cols)
	minimum := make([]int, cols)
	headerMin := make([]int, cols)
	for r, row := range rows {
		out[r] = make([]string, len(row))
		for c, s := range row {
			s = joinSoftBreaks(s, true)
			if r == 0 {
				// 見出しは 1 行で表示されるので、ハード改行は空白にして 1 行にまとめる
				s = strings.ReplaceAll(s, gansi.HardBreak, " ")
			}
			out[r][c] = s
			natural[c] = max(natural[c], maxLineWidth(s))
			minimum[c] = max(minimum[c], maxUnitWidth(s))
			if r == 0 {
				headerMin[c] = maxLineWidth(s)
			}
		}
	}

	avail := width - overhead
	total := 0
	for _, n := range natural {
		total += n
	}
	if total <= avail || avail < 2*cols {
		// 収まるなら折り返さない。
		// 各列に全角 1 文字分 (2 桁) も取れないほど狭い場合は、どう折り返しても全角文字がはみ出すので、
		// 手を出さず lipgloss に任せる (lipgloss は入らない列の内容を落とすことがある。README の既知の制約)
		for r := range out {
			for c := range out[r] {
				out[r][c] = strings.ReplaceAll(out[r][c], gansi.HardBreak, "\n")
			}
		}
		return out
	}

	// 見出しが切れずに収まる幅を下限にできるならそうする。無理なら塊の幅だけを下限にする
	lower := make([]int, cols)
	sumLower := 0
	for c := range lower {
		lower[c] = max(minimum[c], headerMin[c])
		sumLower += min(lower[c], natural[c])
	}
	if sumLower > avail {
		lower = minimum
	}

	colWidths := allocateColumns(natural, lower, avail)
	for r := range out {
		for c := range out[r] {
			if r == 0 {
				// 見出しは折り返さず、入らない分は末尾を切り詰める
				if maxLineWidth(out[r][c]) > colWidths[c] {
					out[r][c] = xansi.Truncate(out[r][c], colWidths[c], "…")
				}
				continue
			}
			out[r][c] = wrapLine(out[r][c], colWidths[c], 0, true)
		}
	}
	return out
}

// maxLineWidth はセルの各行のうち最大の表示幅を返す。
func maxLineWidth(s string) int {
	s = strings.ReplaceAll(stripEscapes(s), gansi.HardBreak, "\n")
	w := 0
	for _, l := range strings.Split(s, "\n") {
		w = max(w, uniseg.StringWidth(l))
	}
	return w
}

// maxUnitWidth は、セルの中で途中で改行できない塊のうち最大の表示幅を返す。
func maxUnitWidth(s string) int {
	cells, _ := splitCells(s)
	w := 0
	for _, u := range buildUnits(cells) {
		if !u.space {
			w = max(w, u.width)
		}
	}
	return w
}

// allocateColumns は合計 avail の幅を列に配る。
//
// 各列の幅は「最小幅 lower」以上「自然な幅 natural」以下に収め、その範囲で共通の水位 L まで
// 揃えて上げる (列幅 = clamp(L, lower, natural))。短い列 (番号・状態など) は自然な幅のまま残り、
// 長い列だけが同じ幅まで削られて折り返される。
//
// lower には、塊 (英単語など) を途中で切らずに済む最小幅 minimum を使う。
// その合計すら avail に収まらない場合は、全角 1 文字が入る 2 桁 (それも無理なら 1 桁) を下限にする。
// (呼び出し側は avail >= 2*列数 のときだけ呼ぶので、実際には 2 桁になる)
func allocateColumns(natural, minimum []int, avail int) []int {
	n := len(natural)
	lower := make([]int, n)
	sumMin := 0
	for i := range natural {
		lower[i] = min(minimum[i], natural[i])
		sumMin += lower[i]
	}
	if sumMin > avail {
		floor := 1
		if avail >= 2*n {
			floor = 2
		}
		for i := range natural {
			lower[i] = min(floor, natural[i])
		}
	}

	at := func(level int) []int {
		ws := make([]int, n)
		for i := range ws {
			ws[i] = min(max(level, lower[i]), max(natural[i], lower[i]))
		}
		return ws
	}
	total := func(ws []int) int {
		t := 0
		for _, w := range ws {
			t += w
		}
		return t
	}

	// 合計が avail を超えない最大の水位を二分探索で求める
	hi := 0
	for _, v := range natural {
		hi = max(hi, v)
	}
	level := sort.Search(hi+1, func(l int) bool { return total(at(l)) > avail }) - 1
	widths := at(max(level, 0))

	// 端数は、まだ自然な幅に達していない列に左から 1 桁ずつ配る
	rest := avail - total(widths)
	for i := 0; i < n && rest > 0; i++ {
		if widths[i] < natural[i] && widths[i] == level {
			widths[i]++
			rest--
		}
	}
	return widths
}
