package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"strings"
)

// 終了コード。スクリプトから失敗の種類を区別できるように分けている。
const (
	exitOK    = 0
	exitError = 1 // ファイルが読めない等の実行時エラー
	exitUsage = 2 // 不正なフラグ等の使い方の誤り (flag パッケージ/シェルの慣習に合わせて 2)
)

// Env はプロセス外部とのやりとりをまとめたもの。
// テストで TTY 判定や環境変数、ページャ起動を差し替えられるように main から注入する。
type Env struct {
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Getenv      func(string) string
	StdinIsTTY  bool
	StdoutIsTTY bool
	TermWidth   int // 0 は不明
	ReadFile    func(string) ([]byte, error)
	// RunPager は content を pagerCmd に流す。ページャを起動できなかった場合は error を返し、
	// 呼び出し側は stdout への直接出力にフォールバックする。
	RunPager func(pagerCmd string, content string, getenv func(string) string) error
}

// Options はコマンドラインから解釈した設定。
type Options struct {
	Paging  string // auto | always | never
	Style   string // glamour のスタイル名 (auto, dark, light, ...)
	Width   int    // 0 は自動
	NoColor bool
	Help    bool
	Version bool
	Files   []string
}

const usageText = `mado - view Markdown in the terminal

Usage:
  mado [options] [FILE...]
  cat FILE | mado

Examples:
  mado README.md            render and page a file
  mado a.md b.md            view several files in a row
  mado -p never plan.md     print without a pager
  git show HEAD:README.md | mado -

Options:
  -p, --paging WHEN   when to use a pager: auto, always, never (default: auto)
  -s, --style NAME    color style: auto, dark, light, dracula, tokyo-night,
                      pink, ascii, notty (default: auto, env: MADO_STYLE)
  -w, --width N       wrap width in columns (default: terminal width, max 100)
      --no-color      disable colors (also honors NO_COLOR and TERM=dumb)
  -h, --help          show this help
      --version       show version

Pager:
  Uses $MADO_PAGER, then $PAGER, then less. When LESS is unset, mado
  runs less with -FIRX: short documents print directly, searches ignore case,
  and the text stays on screen after quitting (q).
`

// 受け付けるスタイル名。glamour 組み込みスタイルと同じ。
var validStyles = []string{"auto", "dark", "light", "dracula", "tokyo-night", "pink", "ascii", "notty"}

// maxAutoWidth は自動決定時の折り返し幅の上限。
// 横長の端末で 1 行が長すぎると読みにくいので、glow の既定 (80) より少し広い程度で止める。
const maxAutoWidth = 100

// errUsage は使い方の誤りを表す。メッセージ表示後に終了コード 2 で終わる。
type errUsage struct{ msg string }

func (e errUsage) Error() string { return e.msg }

// parseArgs は引数を解釈する。
// Go の flag パッケージは最初の非フラグ引数で解釈を止めてしまうため、
// `mado README.md -p never` のようにファイルの後ろにフラグを書いても効くよう、
// 非フラグ引数を拾いながら繰り返し Parse する (clig.dev の「フラグの順序を問わない」に従う)。
func parseArgs(args []string) (Options, error) {
	opts := Options{Paging: "auto", Style: ""}
	fsFlags := flag.NewFlagSet("mado", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard) // エラーメッセージは自前で整形して出す
	fsFlags.Usage = func() {}

	// 短縮形と長形式を同じ変数に束ねる
	fsFlags.StringVar(&opts.Paging, "p", opts.Paging, "")
	fsFlags.StringVar(&opts.Paging, "paging", opts.Paging, "")
	fsFlags.StringVar(&opts.Style, "s", opts.Style, "")
	fsFlags.StringVar(&opts.Style, "style", opts.Style, "")
	fsFlags.IntVar(&opts.Width, "w", 0, "")
	fsFlags.IntVar(&opts.Width, "width", 0, "")
	fsFlags.BoolVar(&opts.NoColor, "no-color", false, "")
	fsFlags.BoolVar(&opts.Help, "h", false, "")
	fsFlags.BoolVar(&opts.Help, "help", false, "")
	fsFlags.BoolVar(&opts.Version, "version", false, "")

	// -h / --help は他にどんな引数があっても (未定義のフラグが混ざっていても) ヘルプを出す。
	// clig.dev: "you should be able to add -h to the end of anything and it should show help"
	// flag パッケージは未定義フラグの時点でエラーを返すので、解釈前に先に探す。"--" 以降はファイル名なので見ない。
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "-h" || a == "--help" || a == "-help" {
			opts.Help = true
			return opts, nil
		}
	}

	rest := args
	for {
		if err := fsFlags.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				// -h は他の引数に関係なくヘルプを出す (flag.ErrHelp は未定義の -help 系で返る)
				opts.Help = true
				return opts, nil
			}
			return opts, errUsage{msg: err.Error()}
		}
		rest = fsFlags.Args()
		if len(rest) == 0 {
			break
		}
		// "--" 以降はすべてファイル名として扱う (flag パッケージは "--" を消費済み)
		if consumedDoubleDash(args, rest) {
			opts.Files = append(opts.Files, rest...)
			break
		}
		opts.Files = append(opts.Files, rest[0])
		rest = rest[1:]
	}

	if opts.Help || opts.Version {
		return opts, nil
	}
	switch opts.Paging {
	case "auto", "always", "never":
	default:
		return opts, errUsage{msg: fmt.Sprintf("invalid --paging value %q (use auto, always, or never)", opts.Paging)}
	}
	if opts.Style != "" && !contains(validStyles, opts.Style) {
		return opts, errUsage{msg: fmt.Sprintf("unknown style %q (use one of: %s)", opts.Style, strings.Join(validStyles, ", "))}
	}
	stdinCount := 0
	for _, f := range opts.Files {
		if f == "-" {
			stdinCount++
		}
	}
	if stdinCount > 1 {
		// stdin は一度読み切ると空になるので、2 回以上の指定は使い方の誤りとして弾く
		return opts, errUsage{msg: "stdin ('-') can only be given once"}
	}
	if opts.Width < 0 {
		return opts, errUsage{msg: fmt.Sprintf("invalid --width value %d (must be 0 or more)", opts.Width)}
	}
	return opts, nil
}

// consumedDoubleDash は、flag パッケージが直前の Parse で "--" を読み飛ばしたかを判定する。
// rest の直前の要素が "--" なら、rest はすべてフラグ解釈の対象外。
func consumedDoubleDash(all, rest []string) bool {
	idx := len(all) - len(rest) - 1
	return idx >= 0 && all[idx] == "--"
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// run はコマンド本体。終了コードを返す。
func run(args []string, env Env) int {
	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(env.Stderr, "mado: %s\nRun 'mado --help' for usage.\n", err)
		return exitUsage
	}
	if opts.Help {
		fmt.Fprint(env.Stdout, usageText)
		return exitOK
	}
	if opts.Version {
		fmt.Fprintf(env.Stdout, "mado %s\n", version)
		return exitOK
	}

	// 引数なし: パイプ入力があれば stdin を読む。対話端末なら待ち続けず使い方を出して終わる。
	if len(opts.Files) == 0 {
		if env.StdinIsTTY {
			fmt.Fprint(env.Stderr, usageText)
			return exitUsage
		}
		opts.Files = []string{"-"}
	}

	docs, err := loadDocuments(opts.Files, env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "mado: %s\n", err)
		return exitError
	}

	color := useColor(opts, env)
	out, err := renderDocuments(docs, RenderConfig{
		Style: resolveStyle(opts, env, color),
		Width: resolveWidth(opts, env),
		Color: color,
		// COLORTERM=truecolor/24bit は 24bit 色対応端末の事実上の標準的な申告方法
		TrueColor: isTrueColor(env.Getenv("COLORTERM")),
	})
	if err != nil {
		fmt.Fprintf(env.Stderr, "mado: render failed: %s\n", err)
		return exitError
	}

	if shouldPage(opts, env) {
		pagerCmd := resolvePager(env.Getenv)
		if pagerCmd != "" {
			if err := env.RunPager(pagerCmd, out, env.Getenv); err == nil {
				return exitOK
			}
			// ページャが起動できなくても内容は見せたいので、stdout に直接出す
		}
	}
	if _, err := io.WriteString(env.Stdout, out); err != nil && !isBrokenPipe(err) {
		fmt.Fprintf(env.Stderr, "mado: %s\n", err)
		return exitError
	}
	return exitOK
}

// Document は読み込んだ 1 ファイル分の内容。
type Document struct {
	Name    string // 表示名。stdin は "-" ではなく "(stdin)"
	Content []byte
}

// loadDocuments は全ファイルを先に読み込む。
// 途中で失敗したときに半端な描画を出さないよう、描画前にすべての入力を検証する。
func loadDocuments(files []string, env Env) ([]Document, error) {
	docs := make([]Document, 0, len(files))
	for _, f := range files {
		if f == "-" {
			// "-" の重複は parseArgs で弾いているので、ここでは高々 1 回しか来ない
			b, err := io.ReadAll(env.Stdin)
			if err != nil {
				return nil, fmt.Errorf("cannot read stdin: %w", err)
			}
			docs = append(docs, Document{Name: "(stdin)", Content: b})
			continue
		}
		b, err := env.ReadFile(f)
		if err != nil {
			return nil, describeReadError(f, err)
		}
		docs = append(docs, Document{Name: f, Content: b})
	}
	return docs, nil
}

// describeReadError は OS のエラーを人間向けの文に直す (clig.dev: "Catch errors and rewrite them for humans")。
func describeReadError(path string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%s: no such file", path)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%s: permission denied", path)
	}
	var pe *fs.PathError
	if errors.As(err, &pe) && strings.Contains(pe.Err.Error(), "is a directory") {
		return fmt.Errorf("%s: is a directory (pass a Markdown file)", path)
	}
	return fmt.Errorf("%s: %w", path, err)
}

// useColor は色付き出力にするかを決める。
// clig.dev の推奨どおり、出力先が TTY でない / NO_COLOR が空でない / TERM=dumb / --no-color のいずれかで無効。
func useColor(opts Options, env Env) bool {
	if opts.NoColor {
		return false
	}
	if env.Getenv("NO_COLOR") != "" {
		return false
	}
	if env.Getenv("TERM") == "dumb" {
		return false
	}
	return env.StdoutIsTTY
}

// resolveStyle は glamour に渡すスタイル名を決める。
// 色を使わない場合は明示指定より優先して notty (装飾なし) にする。
func resolveStyle(opts Options, env Env, color bool) string {
	if !color {
		// ascii は色なしでも成立するスタイルなので、明示指定されたら尊重する
		if opts.Style == "ascii" {
			return "ascii"
		}
		return "notty"
	}
	if opts.Style != "" {
		return opts.Style
	}
	if s := env.Getenv("MADO_STYLE"); s != "" && contains(validStyles, s) {
		return s
	}
	return "auto"
}

// resolveWidth は折り返し幅を決める。
func resolveWidth(opts Options, env Env) int {
	if opts.Width > 0 {
		return opts.Width
	}
	if env.TermWidth <= 0 {
		return 80 // パイプ先など幅が分からない場合は伝統的な 80 桁
	}
	if env.TermWidth > maxAutoWidth {
		return maxAutoWidth
	}
	return env.TermWidth
}

func isTrueColor(colorterm string) bool {
	return colorterm == "truecolor" || colorterm == "24bit"
}

// shouldPage はページャを使うかを決める。
// auto のときは stdout が端末の場合のみ。パイプやリダイレクトにページャを挟むと壊れるため。
func shouldPage(opts Options, env Env) bool {
	switch opts.Paging {
	case "always":
		return true
	case "never":
		return false
	}
	return env.StdoutIsTTY
}

// bytesTrimBOM は UTF-8 BOM を取り除く。Windows 由来のファイルで先頭に余計な記号が出るのを防ぐ。
func bytesTrimBOM(b []byte) []byte {
	return bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
}
