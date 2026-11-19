package ansi

import (
	"bytes"
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/muesli/reflow/indent"
	astext "github.com/yuin/goldmark/extension/ast"
)

// A TableElement is used to render tables.
type TableElement struct {
	lipgloss *table.Table
	table    *astext.Table
	header   []string
	row      []string
	source   []byte

	tableImages []tableLink
	tableLinks  []tableLink

	// mado: TableHook 用。全行 (先頭はヘッダ) を溜めておき、Finish でまとめて整形してから lipgloss に渡す
	pending   [][]string
	wrapWidth int
}

// A TableRowElement is used to render a single row in a table.
type TableRowElement struct{}

// A TableHeadElement is used to render a table's head element.
type TableHeadElement struct{}

// A TableCellElement is used to render a single cell in a row.
type TableCellElement struct {
	Children []ElementRenderer
	Head     bool
}

// Render renders a TableElement.
func (e *TableElement) Render(w io.Writer, ctx RenderContext) error {
	bs := ctx.blockStack

	var indentation uint
	var margin uint
	rules := ctx.options.Styles.Table
	if rules.Indent != nil {
		indentation = *rules.Indent
	}
	if rules.Margin != nil {
		margin = *rules.Margin
	}

	iw := indent.NewWriterPipe(w, indentation+margin, func(_ io.Writer) {
		renderText(w, ctx.options.ColorProfile, bs.Current().Style.StylePrimitive, " ")
	})

	style := bs.With(rules.StylePrimitive)

	renderText(iw, ctx.options.ColorProfile, bs.Current().Style.StylePrimitive, rules.BlockPrefix)
	renderText(iw, ctx.options.ColorProfile, style, rules.Prefix)
	width := int(ctx.blockStack.Width(ctx)) //nolint: gosec

	wrap := true
	if ctx.options.TableWrap != nil {
		wrap = *ctx.options.TableWrap
	}
	ctx.table.lipgloss = table.New().Width(width).Wrap(wrap)
	ctx.table.wrapWidth = width
	ctx.table.pending = nil

	if err := e.collectLinksAndImages(ctx); err != nil {
		return err
	}

	return nil
}

func (e *TableElement) setStyles(ctx RenderContext) {
	ctx.table.lipgloss = ctx.table.lipgloss.StyleFunc(func(_, col int) lipgloss.Style {
		st := lipgloss.NewStyle().Inline(false)
		// Default Styles
		st = st.Margin(0, 1)

		// Override with custom styles
		if m := ctx.options.Styles.Table.Margin; m != nil {
			st = st.Padding(0, int(*m)) //nolint: gosec
		}
		switch e.table.Alignments[col] {
		case astext.AlignLeft:
			st = st.Align(lipgloss.Left).PaddingRight(0)
		case astext.AlignCenter:
			st = st.Align(lipgloss.Center)
		case astext.AlignRight:
			st = st.Align(lipgloss.Right).PaddingLeft(0)
		case astext.AlignNone:
			// do nothing
		}

		return st
	})
}

func (e *TableElement) setBorders(ctx RenderContext) {
	rules := ctx.options.Styles.Table
	border := lipgloss.NormalBorder()

	if rules.RowSeparator != nil && rules.ColumnSeparator != nil {
		border = lipgloss.Border{
			Top:    *rules.RowSeparator,
			Bottom: *rules.RowSeparator,
			Left:   *rules.ColumnSeparator,
			Right:  *rules.ColumnSeparator,
			Middle: *rules.CenterSeparator,
		}
	}
	ctx.table.lipgloss.Border(border)
	ctx.table.lipgloss.BorderTop(false)
	ctx.table.lipgloss.BorderLeft(false)
	ctx.table.lipgloss.BorderRight(false)
	ctx.table.lipgloss.BorderBottom(false)
}

// Finish finishes rendering a TableElement.
func (e *TableElement) Finish(_ io.Writer, ctx RenderContext) error {
	defer func() {
		ctx.table.lipgloss = nil
		ctx.table.tableImages = nil
		ctx.table.tableLinks = nil
	}()

	rules := ctx.options.Styles.Table

	e.setStyles(ctx)
	e.setBorders(ctx)

	// mado: セルを整形してから lipgloss に渡す
	if TableHook != nil && len(ctx.table.pending) > 0 {
		cols := len(ctx.table.pending[0])
		overhead := e.measureOverhead(ctx, cols)
		cells := TableHook(ctx.table.pending, ctx.table.wrapWidth, overhead)
		ctx.table.lipgloss.Headers(cells[0]...)
		for _, row := range cells[1:] {
			ctx.table.lipgloss.Row(row...)
		}
		ctx.table.pending = nil
	}

	ow := ctx.blockStack.Current().Block
	if _, err := ow.WriteString(ctx.table.lipgloss.String()); err != nil {
		return fmt.Errorf("glamour: error writing to buffer: %w", err)
	}

	renderText(ow, ctx.options.ColorProfile, ctx.blockStack.With(rules.StylePrimitive), rules.Suffix)
	renderText(ow, ctx.options.ColorProfile, ctx.blockStack.Current().Style.StylePrimitive, rules.BlockSuffix)

	e.printTableLinks(ctx)

	return nil
}

// measureOverhead は、この表の罫線と余白が占める幅を測る (mado による追加)。
// 同じスタイル・罫線で各セルが幅 1 の表を描いて、セルの幅の合計を引く。
func (e *TableElement) measureOverhead(ctx RenderContext, cols int) int {
	saved := ctx.table.lipgloss
	defer func() { ctx.table.lipgloss = saved }()

	ctx.table.lipgloss = table.New()
	e.setStyles(ctx)
	e.setBorders(ctx)
	probe := make([]string, cols)
	for i := range probe {
		probe[i] = "x"
	}
	ctx.table.lipgloss.Headers(probe...)
	return lipgloss.Width(ctx.table.lipgloss.String()) - cols
}

// Finish finishes rendering a TableRowElement.
func (e *TableRowElement) Finish(_ io.Writer, ctx RenderContext) error {
	if ctx.table.lipgloss == nil {
		return nil
	}

	if TableHook != nil {
		// mado: 表全体の列幅を決めてから整形するので、ここでは溜めるだけ
		ctx.table.pending = append(ctx.table.pending, ctx.table.row)
		ctx.table.row = []string{}
		return nil
	}

	ctx.table.lipgloss.Row(ctx.table.row...)
	ctx.table.row = []string{}
	return nil
}

// Finish finishes rendering a TableHeadElement.
func (e *TableHeadElement) Finish(_ io.Writer, ctx RenderContext) error {
	if ctx.table.lipgloss == nil {
		return nil
	}

	if TableHook != nil {
		// mado: ヘッダは pending の先頭に置く
		ctx.table.pending = append([][]string{ctx.table.header}, ctx.table.pending...)
		ctx.table.header = []string{}
		return nil
	}

	ctx.table.lipgloss.Headers(ctx.table.header...)
	ctx.table.header = []string{}
	return nil
}

// Render renders a TableCellElement.
func (e *TableCellElement) Render(_ io.Writer, ctx RenderContext) error {
	var b bytes.Buffer
	style := ctx.options.Styles.Table.StylePrimitive
	for _, child := range e.Children {
		if r, ok := child.(StyleOverriderElementRenderer); ok {
			if err := r.StyleOverrideRender(&b, ctx, style); err != nil {
				return fmt.Errorf("glamour: error rendering with style: %w", err)
			}
		} else {
			var bb bytes.Buffer
			if err := child.Render(&bb, ctx); err != nil {
				return fmt.Errorf("glamour: error rendering: %w", err)
			}
			el := &BaseElement{
				Token: bb.String(),
				Style: style,
			}
			if err := el.Render(&b, ctx); err != nil {
				return err
			}
		}
	}

	if e.Head {
		ctx.table.header = append(ctx.table.header, b.String())
	} else {
		ctx.table.row = append(ctx.table.row, b.String())
	}

	return nil
}
