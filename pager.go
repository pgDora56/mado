package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// defaultLessFlags は LESS 未設定時に less に渡すフラグ (clig.dev の推奨値)。
//
//	-F: 1 画面に収まるならページャを開かず表示だけして終了
//	-I: 検索で大文字小文字を区別しない
//	-R: 色のエスケープシーケンスをそのまま通す
//	-X: 終了時に画面をクリアしない (閉じた後も内容が見える)
const defaultLessFlags = "FIRX"

// resolvePager は使うページャのコマンド文字列を返す。空文字ならページャを使わない。
// 優先順は MDPEEK_PAGER > PAGER > less。git や bat と同じく、ツール専用の変数を最優先にする。
func resolvePager(getenv func(string) string) string {
	for _, key := range []string{"MDPEEK_PAGER", "PAGER"} {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			if v == "cat" {
				// PAGER=cat は「ページャ不要」の慣習的な指定。cat を起動するより直接書く方が素直
				return ""
			}
			return v
		}
	}
	return "less"
}

// runPager は content をページャの標準入力に流し、ページャの終了を待つ。
// コマンド文字列は git と同様に sh -c で解釈する (PAGER="less -S" のような引数付き指定に対応するため)。
func runPager(pagerCmd string, content string, getenv func(string) string) error {
	cmd := exec.Command("sh", "-c", pagerCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = pagerEnv(pagerCmd, os.Environ(), getenv)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	// ページャ表示中の Ctrl-C は less 側で処理させたい。mdpeek が先に死ぬと端末が壊れた状態で
	// 残ることがあるため、待機中は SIGINT を受け流す。Notify で捕捉する方式なら子プロセスには
	// 既定のハンドラが引き継がれる (Ignore だと SIG_IGN が子に継承されてしまう)。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	if err := cmd.Start(); err != nil {
		return err
	}
	// ユーザーが途中で q を押すとページャが先に終わり、書き込みは EPIPE になる。これは正常系。
	_, werr := io.WriteString(stdin, content)
	stdin.Close()
	waitErr := cmd.Wait()

	if werr != nil && !isBrokenPipe(werr) {
		return werr
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) && exitErr.ExitCode() == 127 {
		// sh がコマンドを見つけられなかった (127)。呼び出し側で stdout に直接出させる
		return waitErr
	}
	// それ以外のページャの終了コード (Ctrl-C 等) は閲覧としては成功扱いにする
	return nil
}

// pagerEnv はページャに渡す環境変数を組み立てる。
// less を使う場合で LESS が未設定なら、色と短文向けのフラグを補う (git と同じ振る舞い)。
// ユーザーが LESS を設定済みならその意思を尊重して触らない。
func pagerEnv(pagerCmd string, base []string, getenv func(string) string) []string {
	env := append([]string{}, base...)
	if isLess(pagerCmd) && getenv("LESS") == "" {
		env = append(env, "LESS="+defaultLessFlags)
	}
	// less 以外でもよく使われる lv は -c で色を通す
	if getenv("LV") == "" {
		env = append(env, "LV=-c")
	}
	return env
}

// isLess はコマンド文字列の先頭が less かを判定する ("/usr/bin/less -S" なども less とみなす)。
func isLess(pagerCmd string) bool {
	fields := strings.Fields(pagerCmd)
	if len(fields) == 0 {
		return false
	}
	return filepath.Base(fields[0]) == "less"
}

// isBrokenPipe は書き込み先が閉じられたことによるエラーかを判定する。
func isBrokenPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE) || errors.Is(err, os.ErrClosed)
}
