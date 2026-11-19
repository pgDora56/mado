package main

import (
	"strings"
	"testing"

	gansi "github.com/charmbracelet/glamour/ansi"
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

func TestFileHeaderTruncatesLongNames(t *testing.T) {
	name := "./999_Drafts/2026-11-20_研究発表_第4回_スライド構成メモ.md"
	h := strings.TrimSuffix(fileHeader(name, RenderConfig{Width: 40}), "\n")
	if w := ansi.StringWidth(h); w != 40 {
		t.Errorf("header width %d, want 40: %q", w, h)
	}
	// 区別に効くファイル名の末尾は残る
	if !strings.Contains(h, "…") || !strings.Contains(h, "メモ.md") {
		t.Errorf("unexpected header: %q", h)
	}
}

func TestTruncateLeft(t *testing.T) {
	if got := truncateLeft("abc", 5); got != "abc" {
		t.Errorf("short string changed: %q", got)
	}
	if got := truncateLeft("abcdefgh", 5); got != "…efgh" {
		t.Errorf("got %q", got)
	}
	// 全角は幅 2 として数える
	if got := truncateLeft("あいうえお", 6); got != "…えお" {
		t.Errorf("got %q", got)
	}
}

func TestRemoveBreakMarks(t *testing.T) {
	in := "a" + gansi.HardBreak + "b" + gansi.SoftBreak + "c"
	if got := removeBreakMarks(in); got != "a\nb c" {
		t.Errorf("got %q", got)
	}
}
