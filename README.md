# mdpeek

Markdown をターミナルでサッと確認するための閲覧専用ビューア。
Claude Code が書き出した計画書・レポート・メモを、エディタを開かずに読む用途を想定している。

- 見出し・強調・リスト・表・引用を色付きで描画
- コードブロックはシンタックスハイライト
- YAML front matter（Claude Code のメモリやスキル定義に付いているもの）はコードブロックとして表示
- 空白のない日本語の段落も端末幅で折り返す
- 長い文書は `less` で開き、1 画面に収まる文書はそのまま表示して終わる

描画には [glamour](https://github.com/charmbracelet/glamour)（glow と同じエンジン）を使っている。

## インストール

Go 1.26 以降が必要（依存する golang.org/x/term の要件）。

```sh
git clone git@github.com:pgDora56/mdpeek.git
cd mdpeek
go install .
```

`$(go env GOPATH)/bin` に PATH が通っていれば `mdpeek` で起動できる。

## 使い方

```sh
mdpeek README.md                  # 描画してページャで開く
mdpeek a.md b.md                  # 複数ファイルを続けて表示 (ファイル名の区切り行付き)
mdpeek -p never plan.md           # ページャを使わずに出力
git show HEAD:README.md | mdpeek  # パイプで渡す (明示するなら "-")
```

ページャ（既定は `less`）の主な操作:

| キー | 動作 |
|---|---|
| `q` | 閉じる |
| `j` / `k`, `↓` / `↑` | 1 行スクロール |
| `Space` / `b` | 1 画面進む / 戻る |
| `g` / `G` | 先頭 / 末尾へ |
| `/文字列` → `n` / `N` | 検索 → 次 / 前の一致 |

### オプション

| オプション | 説明 | 既定値 |
|---|---|---|
| `-p, --paging WHEN` | ページャを使うか: `auto` / `always` / `never` | `auto`（出力先が端末のときだけ） |
| `-s, --style NAME` | 配色: `auto` `dark` `light` `dracula` `tokyo-night` `pink` `ascii` `notty` | `auto`（端末の背景色から判定） |
| `-w, --width N` | 折り返し幅 | 端末幅（上限 100） |
| `--no-color` | 色を付けない | |
| `-h, --help` | ヘルプ | |
| `--version` | バージョン | |

フラグはファイル名の前後どちらに書いてもよい。`--` 以降はすべてファイル名として扱う。

### 環境変数

| 変数 | 効果 |
|---|---|
| `MDPEEK_PAGER` | 使うページャ。`PAGER` より優先 |
| `PAGER` | 使うページャ。`cat` を指定するとページャを使わない |
| `LESS` | 未設定なら mdpeek が `FIRX` を設定して less を起動する。設定済みならそのまま使う（色を出すには `R` を含めること） |
| `MDPEEK_STYLE` | `--style` の既定値 |
| `NO_COLOR` | 空でなければ色を付けない |

### 終了コード

| コード | 意味 |
|---|---|
| 0 | 成功 |
| 1 | ファイルが読めない等の実行時エラー |
| 2 | 不正なオプション等の使い方の誤り |

## 設計方針

CLI の一般的な作法は [Command Line Interface Guidelines (clig.dev)](https://clig.dev/) に、
ページャまわりの振る舞いは git・bat・glow といった既存ツールの慣習に合わせている。

| 項目 | 振る舞い | 根拠 |
|---|---|---|
| ページャ | 出力先が端末のときだけ `less -FIRX` に流す。パイプやリダイレクトでは使わない | clig.dev が推奨する less のオプション。`-F` で短い文書はページャを開かない |
| 色 | 出力先が端末でない / `NO_COLOR` が空でない / `TERM=dumb` / `--no-color` のいずれかで無効 | clig.dev の色の無効化条件 |
| 標準入力 | ファイル名に `-` を指定するか、引数なしでパイプ入力すると stdin を読む。引数なしで端末から起動した場合は入力を待たずに使い方を表示して終了 | clig.dev |
| ヘルプ | `-h` / `--help` は他の引数があっても優先してヘルプを出す。冒頭に使用例を載せる | clig.dev |
| バージョン | `--version` のみ。`-v` は割り当てない | clig.dev（`-v` は verbose と混同されやすい） |
| エラー | stderr に `mdpeek: <対象>: <何が起きたか>` の形で 1 行。使い方の誤りでは `--help` を案内 | clig.dev |
| ページャの選択 | `MDPEEK_PAGER` → `PAGER` → `less` の順。コマンド文字列は `sh -c` で解釈 | git（`GIT_PAGER` → `PAGER` → `less`）と同じ考え方 |
| 複数ファイル | ファイル名入りの区切り行を挟んで連結 | bat |

## 既知の制約

- 空白なしで日本語に直結した URL（`[リンク](https://example.com)。続く文…`）は、URL の途中で改行されることがある（glamour 側の折り返しの挙動）
- `--style auto` は端末に背景色を問い合わせる。対応していない端末では判定に少し時間がかかったり、dark になったりする。気になる場合は `MDPEEK_STYLE` で固定する

## 開発

```sh
go test ./...
go build -o mdpeek .
```
