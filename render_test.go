package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFrontMatterToCodeBlock(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			name: "front matter becomes yaml block",
			in:   "---\nname: x\n---\n# Body\n",
			want: "```yaml\nname: x\n```\n\n# Body\n",
		},
		{
			name: "front matter at end of file",
			in:   "---\nname: x\n---",
			want: "```yaml\nname: x\n```\n\n",
		},
		{
			name: "empty front matter",
			in:   "---\n---\nbody",
			want: "```yaml\n```\n\nbody",
		},
		{
			name: "no closing delimiter is left alone",
			in:   "---\nnot front matter\n",
			want: "---\nnot front matter\n",
		},
		{
			name: "horizontal rule in the middle is untouched",
			in:   "text\n---\nmore\n",
			want: "text\n---\nmore\n",
		},
		{
			name: "dashes inside a value do not close",
			in:   "---\na: ----x\n---\nb",
			want: "```yaml\na: ----x\n```\n\nb",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(frontMatterToCodeBlock([]byte(tt.in))); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPreprocessNormalizesBOMAndCRLF(t *testing.T) {
	got := string(preprocess([]byte("\xef\xbb\xbf---\r\nk: v\r\n---\r\nbody\r\n")))
	want := "```yaml\nk: v\n```\n\nbody\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRewrapOverflowJapanese(t *testing.T) {
	// 空白のない日本語段落は glamour だけでは折り返されない。描画後の全行が幅に収まることを確認する
	long := strings.Repeat("日本語の長い段落です。", 20)
	for _, color := range []bool{false, true} {
		style := "notty"
		if color {
			style = "dark"
		}
		out, err := renderDocuments([]Document{{Name: "a", Content: []byte(long)}},
			RenderConfig{Style: style, Width: 40, Color: color})
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(out, "\n") {
			if w := ansi.StringWidth(line); w > 40 {
				t.Errorf("color=%v: line too wide (%d): %q", color, w, ansi.Strip(line))
			}
		}
		// 折り返しで文字が欠けていないこと
		plain := strings.ReplaceAll(ansi.Strip(out), " ", "")
		plain = strings.ReplaceAll(plain, "\n", "")
		if plain != long {
			t.Errorf("color=%v: text changed after wrapping:\n%q", color, plain)
		}
	}
}

func TestRewrapOverflowKeepsIndent(t *testing.T) {
	in := "    " + strings.Repeat("あ", 30)
	out := rewrapOverflow(in, 20)
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %q", out)
	}
	for i, l := range lines {
		if !strings.HasPrefix(l, "    ") {
			t.Errorf("line %d lost indent: %q", i, l)
		}
		if ansi.StringWidth(l) > 20 {
			t.Errorf("line %d too wide: %q", i, l)
		}
	}
}

func TestRewrapOverflowLeavesShortLines(t *testing.T) {
	in := "  short\n  line"
	if got := rewrapOverflow(in, 40); got != in {
		t.Errorf("got %q", got)
	}
}

func TestTrimTrailingSpaces(t *testing.T) {
	if got := trimTrailingSpaces("abc   "); got != "abc" {
		t.Errorf("plain: got %q", got)
	}
	// 色付き行は末尾にリセットを付けて色を閉じる
	got := trimTrailingSpaces("\x1b[1mabc\x1b[0m   ")
	if ansi.Strip(got) != "abc" || !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("colored: got %q", got)
	}
}

func TestRenderNoColorHasNoEscapes(t *testing.T) {
	out, err := renderDocuments([]Document{{Name: "a", Content: []byte("# T\n\n```go\nfunc f() {}\n```\n")}},
		RenderConfig{Style: "notty", Width: 80, Color: false})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\x1b") {
		t.Errorf("unexpected escape sequence: %q", out)
	}
}

func TestRenderColorHighlightsCode(t *testing.T) {
	out, err := renderDocuments([]Document{{Name: "a", Content: []byte("```go\nfunc f() {}\n```\n")}},
		RenderConfig{Style: "dark", Width: 80, Color: true})
	if err != nil {
		t.Fatal(err)
	}
	// "func" キーワードが色付きで出ていればシンタックスハイライトが効いている
	if !strings.Contains(out, "func\x1b[0m") {
		t.Errorf("code is not highlighted: %q", out)
	}
}

func TestFileHeader(t *testing.T) {
	h := fileHeader("日本語.md", RenderConfig{Width: 30})
	if w := ansi.StringWidth(strings.TrimSuffix(h, "\n")); w != 30 {
		t.Errorf("header width %d, want 30: %q", w, h)
	}
	if strings.Contains(h, "\x1b") {
		t.Errorf("no-color header must be plain: %q", h)
	}
}
