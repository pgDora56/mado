package ansi

// このファイルは mado による追加 (上流の glamour には存在しない)。
//
// glamour は段落・見出し・ブロック (リスト等) の折り返しを空白でしか行わないため、
// 空白を含まない日本語が 1 行のまま端末幅を突き抜けたり、英単語と日本語の間の空白で
// 早めに改行されたりする。折り返し処理を外から差し替えられるよう、フックを用意する。

// WrapMode は折り返し対象の種類。
type WrapMode int

const (
	// WrapParagraph は段落本文。"\n" はソフト改行 (結合してよい)、HardBreak は強制改行。
	WrapParagraph WrapMode = iota
	// WrapHeading は見出し。
	WrapHeading
	// WrapBlock はリスト・引用・文書全体などのブロック。"\n" は行の区切りとしてそのまま保つ。
	WrapBlock
)

// HardBreak は WrapHook 設定時に、Markdown のハード改行 (行末の空白 2 つ / バックスラッシュ) を
// ソフト改行と区別するために使う目印 (U+2028 LINE SEPARATOR)。WrapHook が必ず "\n" に置き換える。
const HardBreak = " "

// WrapHook が nil でなければ、上流の reflow / ansi.Wordwrap による折り返しの代わりに呼ばれる。
// s は ANSI エスケープを含む描画済みテキスト、width は表示幅の上限。
var WrapHook func(s string, width int, mode WrapMode) string
