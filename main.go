// mdpeek は Markdown をターミナルでサッと確認するための閲覧専用ビューア。
//
// 設計方針は Command Line Interface Guidelines (https://clig.dev/) と
// bat / glow / git といった既存 CLI の慣習に寄せている:
//   - 出力が TTY のときだけ色を付け、ページャ (既定は less -FIRX) に流す
//   - パイプ先に出すときは装飾なしのテキストを stdout に書く
//   - NO_COLOR / TERM=dumb / --no-color で色を止められる
//   - エラーは stderr、終了コードは 0=成功 / 1=実行時エラー / 2=使い方の誤り
package main

import (
	"os"

	"golang.org/x/term"
)

// version はリリースビルド時に -ldflags "-X main.version=..." で差し替える。
var version = "dev"

func main() {
	env := Env{
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Getenv:      os.Getenv,
		StdinIsTTY:  term.IsTerminal(int(os.Stdin.Fd())),
		StdoutIsTTY: term.IsTerminal(int(os.Stdout.Fd())),
		TermWidth:   terminalWidth(),
		ReadFile:    os.ReadFile,
		RunPager:    runPager,
	}
	os.Exit(run(os.Args[1:], env))
}

// terminalWidth は stdout の端末幅を返す。取れない場合 (パイプ等) は 0。
func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}
	return w
}
