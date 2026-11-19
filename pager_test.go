package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestResolvePager(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
		want string
	}{
		{name: "default is less", want: "less"},
		{name: "PAGER", vars: map[string]string{"PAGER": "most"}, want: "most"},
		{name: "MDPEEK_PAGER wins", vars: map[string]string{"PAGER": "most", "MDPEEK_PAGER": "less -S"}, want: "less -S"},
		{name: "PAGER=cat disables paging", vars: map[string]string{"PAGER": "cat"}, want: ""},
		{name: "blank values are ignored", vars: map[string]string{"PAGER": "  "}, want: "less"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolvePager(func(k string) string { return tt.vars[k] }); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPagerEnv(t *testing.T) {
	none := func(string) string { return "" }
	env := pagerEnv("less", nil, none)
	if !slices.Contains(env, "LESS=FIRX") {
		t.Errorf("LESS should be set for less: %v", env)
	}
	env = pagerEnv("/usr/bin/less -S", nil, none)
	if !slices.Contains(env, "LESS=FIRX") {
		t.Errorf("LESS should be set for a less path with args: %v", env)
	}
	// ユーザーが LESS を設定済みなら上書きしない
	env = pagerEnv("less", []string{"LESS=M"}, func(k string) string {
		if k == "LESS" {
			return "M"
		}
		return ""
	})
	if slices.Contains(env, "LESS=FIRX") {
		t.Errorf("user LESS must be respected: %v", env)
	}
	env = pagerEnv("most", nil, none)
	if slices.Contains(env, "LESS=FIRX") {
		t.Errorf("LESS should not be added for other pagers: %v", env)
	}
}

func TestRunPagerPassesContent(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.txt")
	// ページャの代わりに標準入力をファイルに書き出すコマンドで、内容がそのまま渡ることを確かめる
	if err := runPager("cat > '"+out+"'", "hello\nworld\n", os.Getenv); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello\nworld\n" {
		t.Errorf("got %q", b)
	}
}

func TestRunPagerEarlyExitIsNotAnError(t *testing.T) {
	// ページャが入力を読み切らずに終了しても (less で q を押した状態) エラーにしない
	big := make([]byte, 1<<20)
	for i := range big {
		big[i] = 'x'
	}
	if err := runPager("true", string(big), os.Getenv); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunPagerMissingCommand(t *testing.T) {
	// 存在しないページャは error を返し、呼び出し側が stdout へフォールバックできるようにする
	if err := runPager("mdpeek-no-such-pager-xyz", "x", os.Getenv); err == nil {
		t.Error("expected an error for a missing pager")
	}
}
