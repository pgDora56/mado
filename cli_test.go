package main

import (
	"bytes"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

// fakeEnv はテスト用の Env を組み立てる。ファイルは files マップから読み、ページャ呼び出しは記録する。
type fakeEnv struct {
	stdout, stderr bytes.Buffer
	vars           map[string]string
	files          map[string]string
	pagerCalls     []string // 渡されたページャコマンド
	pagerContent   string
	pagerErr       error
}

func (f *fakeEnv) env(stdin string, stdinTTY, stdoutTTY bool) Env {
	return Env{
		Stdin:       strings.NewReader(stdin),
		Stdout:      &f.stdout,
		Stderr:      &f.stderr,
		Getenv:      func(k string) string { return f.vars[k] },
		StdinIsTTY:  stdinTTY,
		StdoutIsTTY: stdoutTTY,
		TermWidth:   80,
		ReadFile: func(p string) ([]byte, error) {
			s, ok := f.files[p]
			if !ok {
				return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
			}
			return []byte(s), nil
		},
		RunPager: func(cmd, content string, _ func(string) string) error {
			f.pagerCalls = append(f.pagerCalls, cmd)
			f.pagerContent = content
			return f.pagerErr
		},
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    Options
		wantErr bool
	}{
		{
			name: "defaults",
			args: nil,
			want: Options{Paging: "auto"},
		},
		{
			name: "flags after file are still parsed",
			args: []string{"a.md", "-p", "never", "b.md", "--width", "60"},
			want: Options{Paging: "never", Width: 60, Files: []string{"a.md", "b.md"}},
		},
		{
			name: "long and short forms",
			args: []string{"--paging=always", "-s", "light", "--no-color", "x.md"},
			want: Options{Paging: "always", Style: "light", NoColor: true, Files: []string{"x.md"}},
		},
		{
			name: "double dash makes the rest file names",
			args: []string{"-p", "never", "--", "-weird.md", "--help"},
			want: Options{Paging: "never", Files: []string{"-weird.md", "--help"}},
		},
		{
			name: "dash means stdin",
			args: []string{"-"},
			want: Options{Paging: "auto", Files: []string{"-"}},
		},
		{
			name: "help anywhere",
			args: []string{"a.md", "-h"},
			want: Options{Paging: "auto", Help: true},
		},
		{
			name: "help wins over unknown flags",
			args: []string{"--bogus", "-h"},
			want: Options{Paging: "auto", Help: true},
		},
		{
			name: "help after double dash is a file name",
			args: []string{"--", "-h"},
			want: Options{Paging: "auto", Files: []string{"-h"}},
		},
		{
			name: "version",
			args: []string{"--version"},
			want: Options{Paging: "auto", Version: true},
		},
		{name: "invalid paging", args: []string{"-p", "sometimes"}, wantErr: true},
		{name: "invalid style", args: []string{"--style", "neon"}, wantErr: true},
		{name: "negative width", args: []string{"-w", "-1"}, wantErr: true},
		{name: "unknown flag", args: []string{"--bogus"}, wantErr: true},
		{name: "stdin twice", args: []string{"-", "-"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if tt.wantErr {
				var ue errUsage
				if !errors.As(err, &ue) {
					t.Fatalf("want errUsage, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestUseColor(t *testing.T) {
	tests := []struct {
		name    string
		noColor bool
		vars    map[string]string
		tty     bool
		want    bool
	}{
		{name: "tty", tty: true, want: true},
		{name: "not a tty", tty: false, want: false},
		{name: "--no-color", noColor: true, tty: true, want: false},
		{name: "NO_COLOR set", vars: map[string]string{"NO_COLOR": "1"}, tty: true, want: false},
		{name: "NO_COLOR empty is ignored", vars: map[string]string{"NO_COLOR": ""}, tty: true, want: true},
		{name: "TERM=dumb", vars: map[string]string{"TERM": "dumb"}, tty: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := Env{Getenv: func(k string) string { return tt.vars[k] }, StdoutIsTTY: tt.tty}
			if got := useColor(Options{NoColor: tt.noColor}, env); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveStyle(t *testing.T) {
	get := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	tests := []struct {
		name  string
		style string
		vars  map[string]string
		color bool
		want  string
	}{
		{name: "auto by default", color: true, want: "auto"},
		{name: "explicit style", style: "light", color: true, want: "light"},
		{name: "env style", vars: map[string]string{"MDPEEK_STYLE": "dracula"}, color: true, want: "dracula"},
		{name: "flag beats env", style: "light", vars: map[string]string{"MDPEEK_STYLE": "dracula"}, color: true, want: "light"},
		{name: "invalid env is ignored", vars: map[string]string{"MDPEEK_STYLE": "neon"}, color: true, want: "auto"},
		{name: "no color forces notty", style: "dark", color: false, want: "notty"},
		{name: "ascii is kept without color", style: "ascii", color: false, want: "ascii"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveStyle(Options{Style: tt.style}, Env{Getenv: get(tt.vars)}, tt.color)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveWidth(t *testing.T) {
	tests := []struct {
		flag, term, want int
	}{
		{flag: 0, term: 0, want: 80},    // 幅不明
		{flag: 0, term: 72, want: 72},   // 端末幅に従う
		{flag: 0, term: 200, want: 100}, // 上限で止める
		{flag: 140, term: 80, want: 140},
	}
	for _, tt := range tests {
		if got := resolveWidth(Options{Width: tt.flag}, Env{TermWidth: tt.term}); got != tt.want {
			t.Errorf("flag=%d term=%d: got %d, want %d", tt.flag, tt.term, got, tt.want)
		}
	}
}

func TestShouldPage(t *testing.T) {
	tests := []struct {
		paging string
		tty    bool
		want   bool
	}{
		{"auto", true, true},
		{"auto", false, false},
		{"always", false, true},
		{"never", true, false},
	}
	for _, tt := range tests {
		if got := shouldPage(Options{Paging: tt.paging}, Env{StdoutIsTTY: tt.tty}); got != tt.want {
			t.Errorf("paging=%s tty=%v: got %v, want %v", tt.paging, tt.tty, got, tt.want)
		}
	}
}

func TestRunHelpAndVersion(t *testing.T) {
	f := &fakeEnv{}
	if code := run([]string{"--help"}, f.env("", true, true)); code != exitOK {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(f.stdout.String(), "Usage:") {
		t.Errorf("help not printed to stdout: %q", f.stdout.String())
	}

	f = &fakeEnv{}
	if code := run([]string{"--version"}, f.env("", true, true)); code != exitOK {
		t.Fatalf("exit code %d", code)
	}
	if !strings.HasPrefix(f.stdout.String(), "mdpeek ") {
		t.Errorf("version output: %q", f.stdout.String())
	}
}

func TestRunNoArgsOnTerminalShowsUsage(t *testing.T) {
	// 対話端末で引数なし → stdin を待ってハングせず、使い方を stderr に出して 2 で終わる
	f := &fakeEnv{}
	code := run(nil, f.env("", true, true))
	if code != exitUsage {
		t.Fatalf("exit code %d, want %d", code, exitUsage)
	}
	if !strings.Contains(f.stderr.String(), "Usage:") || f.stdout.Len() != 0 {
		t.Errorf("stdout=%q stderr=%q", f.stdout.String(), f.stderr.String())
	}
}

func TestRunReadsStdinWhenPiped(t *testing.T) {
	f := &fakeEnv{}
	code := run(nil, f.env("# Title\n\nhello", false, false))
	if code != exitOK {
		t.Fatalf("exit code %d, stderr=%q", code, f.stderr.String())
	}
	out := f.stdout.String()
	if !strings.Contains(out, "Title") || !strings.Contains(out, "hello") {
		t.Errorf("output: %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("piped output must not contain escape sequences: %q", out)
	}
	if len(f.pagerCalls) != 0 {
		t.Errorf("pager must not be used when stdout is not a tty")
	}
}

func TestRunMissingFile(t *testing.T) {
	f := &fakeEnv{files: map[string]string{"ok.md": "# ok"}}
	code := run([]string{"ok.md", "missing.md"}, f.env("", true, false))
	if code != exitError {
		t.Fatalf("exit code %d, want %d", code, exitError)
	}
	if got := f.stderr.String(); got != "mdpeek: missing.md: no such file\n" {
		t.Errorf("stderr: %q", got)
	}
	// 1 つでも読めなければ何も出力しない (半端な描画を出さない)
	if f.stdout.Len() != 0 {
		t.Errorf("stdout should be empty: %q", f.stdout.String())
	}
}

func TestRunUsageError(t *testing.T) {
	f := &fakeEnv{}
	code := run([]string{"--paging", "x"}, f.env("", true, true))
	if code != exitUsage {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(f.stderr.String(), "mdpeek --help") {
		t.Errorf("stderr should point to --help: %q", f.stderr.String())
	}
}

func TestRunStdinTwice(t *testing.T) {
	f := &fakeEnv{}
	if code := run([]string{"-", "-"}, f.env("x", false, false)); code != exitUsage {
		t.Fatalf("exit code %d", code)
	}
}

func TestRunMultipleFilesHaveHeaders(t *testing.T) {
	f := &fakeEnv{files: map[string]string{"a.md": "alpha", "b.md": "beta"}}
	if code := run([]string{"a.md", "b.md"}, f.env("", true, false)); code != exitOK {
		t.Fatalf("exit code %d", code)
	}
	out := f.stdout.String()
	ia, ib := strings.Index(out, "── a.md"), strings.Index(out, "── b.md")
	if ia < 0 || ib < 0 || ia > ib {
		t.Errorf("headers missing or out of order: %q", out)
	}
}

func TestRunUsesPagerOnTerminal(t *testing.T) {
	f := &fakeEnv{
		files: map[string]string{"a.md": "# Hi"},
		vars:  map[string]string{"PAGER": "most"},
	}
	// -s dark: auto だと glamour が実端末に背景色を問い合わせるので固定する
	if code := run([]string{"-s", "dark", "a.md"}, f.env("", true, true)); code != exitOK {
		t.Fatalf("exit code %d", code)
	}
	if !reflect.DeepEqual(f.pagerCalls, []string{"most"}) {
		t.Errorf("pager calls: %v", f.pagerCalls)
	}
	if !strings.Contains(f.pagerContent, "\x1b[") {
		t.Errorf("tty output should be colored")
	}
	if f.stdout.Len() != 0 {
		t.Errorf("content should go to the pager, not stdout")
	}
}

func TestRunFallsBackToStdoutWhenPagerFails(t *testing.T) {
	f := &fakeEnv{
		files:    map[string]string{"a.md": "# Hi"},
		pagerErr: errors.New("exec: not found"),
	}
	if code := run([]string{"-s", "dark", "a.md"}, f.env("", true, true)); code != exitOK {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(f.stdout.String(), "Hi") {
		t.Errorf("content should be printed directly: %q", f.stdout.String())
	}
}

func TestDescribeReadError(t *testing.T) {
	err := describeReadError("x.md", &fs.PathError{Op: "open", Path: "x.md", Err: fs.ErrPermission})
	if err.Error() != "x.md: permission denied" {
		t.Errorf("got %q", err)
	}
}
