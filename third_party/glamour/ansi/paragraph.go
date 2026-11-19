package ansi

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/muesli/reflow/wordwrap"
)

// A ParagraphElement is used to render individual paragraphs.
type ParagraphElement struct {
	First bool
}

// Render renders a ParagraphElement.
func (e *ParagraphElement) Render(w io.Writer, ctx RenderContext) error {
	bs := ctx.blockStack
	rules := ctx.options.Styles.Paragraph

	if !e.First {
		_, _ = io.WriteString(w, "\n")
	}
	be := BlockElement{
		Block: &bytes.Buffer{},
		Style: cascadeStyle(bs.Current().Style, rules, false),
	}
	bs.Push(be)

	renderText(w, ctx.options.ColorProfile, bs.Parent().Style.StylePrimitive, rules.BlockPrefix)
	renderText(bs.Current().Block, ctx.options.ColorProfile, bs.Current().Style.StylePrimitive, rules.Prefix)
	return nil
}

// Finish finishes rendering a ParagraphElement.
func (e *ParagraphElement) Finish(w io.Writer, ctx RenderContext) error {
	bs := ctx.blockStack
	rules := bs.Current().Style

	mw := NewMarginWriter(ctx, w, rules)
	if len(strings.TrimSpace(bs.Current().Block.String())) > 0 {
		var wrapped []byte
		if WrapHook != nil {
			// mado: 日本語向けの折り返しに差し替え
			wrapped = []byte(WrapHook(bs.Current().Block.String(), int(bs.Width(ctx)), WrapParagraph)) //nolint: gosec
		} else {
			flow := wordwrap.NewWriter(int(bs.Width(ctx))) //nolint: gosec
			flow.KeepNewlines = ctx.options.PreserveNewLines
			_, _ = flow.Write(bs.Current().Block.Bytes())
			if err := flow.Close(); err != nil {
				return fmt.Errorf("glamour: error closing flow: %w", err)
			}
			wrapped = flow.Bytes()
		}

		_, err := mw.Write(wrapped)
		if err != nil {
			return err
		}
		_, _ = io.WriteString(mw, "\n")
	}

	renderText(w, ctx.options.ColorProfile, bs.Current().Style.StylePrimitive, rules.Suffix)
	renderText(w, ctx.options.ColorProfile, bs.Parent().Style.StylePrimitive, rules.BlockSuffix)

	bs.Current().Block.Reset()
	bs.Pop()
	return nil
}
