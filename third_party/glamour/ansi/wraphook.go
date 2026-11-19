package ansi

// このファイルは mado による追加 (上流の glamour には存在しない)。
//
// glamour は段落・見出し・ブロック (リスト等)・表のセルの折り返しを空白でしか行わないため、
// 空白を含まない日本語が 1 行のまま端末幅を突き抜けたり、英単語と日本語の間の空白で
// 早めに改行されたりする。折り返し処理を外から差し替えられるよう、フックを用意する。

// WrapMode は折り返し対象の種類。
type WrapMode int

const (
	// WrapParagraph は段落本文。SoftBreak は結合してよい改行、HardBreak は強制改行。
	WrapParagraph WrapMode = iota
	// WrapHeading は見出し。
	WrapHeading
	// WrapBlock はリスト・引用・文書全体などのブロック。"\n" は行の区切りとしてそのまま保つ。
	// 行の中に SoftBreak / HardBreak があれば、それはリスト項目内の改行。
	WrapBlock
)

// HardBreak は WrapHook 設定時に、Markdown のハード改行 (行末の空白 2 つ / バックスラッシュ) を
// 表すために使う目印 (U+2028 LINE SEPARATOR)。WrapHook が必ず改行に置き換える。
const HardBreak = " "

// SoftBreak は WrapHook 設定時に、Markdown のソフト改行 (段落内の単なる改行) を表す目印
// (U+2029 PARAGRAPH SEPARATOR)。上流は "\n" を出すが、それだとリストのブロック内で
// 「項目内の改行」と「項目同士の区切り」が区別できないため、目印で分ける。
// WrapHook が必ず空白か空文字に置き換える。
const SoftBreak = " "

// WrapHook が nil でなければ、上流の reflow / ansi.Wordwrap による折り返しの代わりに呼ばれる。
// s は ANSI エスケープを含む描画済みテキスト、width は表示幅の上限。
var WrapHook func(s string, width int, mode WrapMode) string

// TableHook が nil でなければ、表のセルを lipgloss に渡す前に呼ばれる。
// cells は 1 行目がヘッダの全セル (描画済みテキスト)、width は表全体の幅の上限、
// overhead は罫線と余白で使われる幅の合計。戻り値のセルは改行を含めてよい
// (lipgloss は改行を保ったまま行の高さを揃える)。
var TableHook func(cells [][]string, width, overhead int) [][]string
