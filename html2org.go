package html2org

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/olekukonko/tablewriter"
	"github.com/ssor/bom"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Options provide toggles and overrides to control specific rendering behaviors.
type Options struct {
	PrettyTables        bool                 // Turns on pretty ASCII rendering for table elements.
	PrettyTablesOptions *PrettyTablesOptions // Configures pretty ASCII rendering for table elements.
	OmitLinks           bool                 // Turns on omitting links
	BreakLongLines      bool
	BaseURL             string
}

// PrettyTablesOptions overrides tablewriter behaviors
type PrettyTablesOptions struct {
	AutoFormatHeader     bool
	AutoWrapText         bool
	ReflowDuringAutoWrap bool
	ColWidth             int
	ColumnSeparator      string
	RowSeparator         string
	CenterSeparator      string
	HeaderAlignment      int
	FooterAlignment      int
	Alignment            int
	ColumnAlignment      []int
	NewLine              string
	HeaderLine           bool
	RowLine              bool
	AutoMergeCells       bool
	Borders              tablewriter.Border
	OrgFormat            bool
}

// NewPrettyTablesOptions creates PrettyTablesOptions with default settings
func NewPrettyTablesOptions() *PrettyTablesOptions {
	return &PrettyTablesOptions{
		AutoFormatHeader:     true,
		AutoWrapText:         true,
		ReflowDuringAutoWrap: true,
		ColWidth:             tablewriter.MAX_ROW_WIDTH,
		ColumnSeparator:      tablewriter.COLUMN,
		RowSeparator:         tablewriter.ROW,
		CenterSeparator:      tablewriter.CENTER,
		HeaderAlignment:      tablewriter.ALIGN_DEFAULT,
		FooterAlignment:      tablewriter.ALIGN_DEFAULT,
		Alignment:            tablewriter.ALIGN_DEFAULT,
		ColumnAlignment:      []int{},
		NewLine:              tablewriter.NEWLINE,
		HeaderLine:           true,
		RowLine:              false,
		AutoMergeCells:       false,
		Borders:              tablewriter.Border{Left: true, Right: true, Bottom: true, Top: true},
		OrgFormat:            true,
	}
}

// FromHTMLNode renders text output from a pre-parsed HTML document.
func FromHTMLNode(doc *html.Node, o ...Options) (string, error) {
	var options Options
	if len(o) > 0 {
		options = o[0]
	}

	ctx := textifyTraverseContext{
		buf:     bytes.Buffer{},
		options: options,
	}
	if err := ctx.traverse(doc); err != nil {
		return "", err
	}

	text := strings.TrimSpace(newlineRe.ReplaceAllString(
		strings.Replace(ctx.buf.String(), "\n ", "\n", -1), "\n\n"),
	)
	return text, nil
}

// FromReader renders text output after parsing HTML for the specified
// io.Reader.
func FromReader(reader io.Reader, options ...Options) (string, error) {
	newReader, err := bom.NewReaderWithoutBom(reader)
	if err != nil {
		return "", err
	}
	doc, err := html.Parse(newReader)
	if err != nil {
		return "", err
	}
	return FromHTMLNode(doc, options...)
}

// FromString parses HTML from the input string, then renders the text form.
func FromString(input string, options ...Options) (string, error) {
	bs := bom.CleanBom([]byte(input))
	text, err := FromReader(bytes.NewReader(bs), options...)
	if err != nil {
		return "", err
	}
	return text, nil
}

var (
	spacingRe = regexp.MustCompile(`[ \r\n\t]+`)
	newlineRe = regexp.MustCompile(`\n\n+`)
)

// traverseTableCtx holds text-related context.
type textifyTraverseContext struct {
	buf bytes.Buffer

	prefix          string
	tableCtx        tableTraverseContext
	options         Options
	endsWithSpace   bool
	endsWithNewLine bool
	justClosedDiv   bool
	blockquoteLevel int
	lineLength      int
	isPre           bool
}

// tableTraverseContext holds table ASCII-form related context.
type tableTraverseContext struct {
	header     []string
	body       [][]string
	footer     []string
	tmpRow     int
	isInFooter bool
}

func (tableCtx *tableTraverseContext) init() {
	tableCtx.body = [][]string{}
	tableCtx.header = []string{}
	tableCtx.footer = []string{}
	tableCtx.isInFooter = false
	tableCtx.tmpRow = 0
}

func (ctx *textifyTraverseContext) handleElement(node *html.Node) error {
	ctx.justClosedDiv = false

	switch node.DataAtom {
	case atom.Br:
		return ctx.emit("\n")

	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		subCtx := textifyTraverseContext{
			options: ctx.options,
		}
		if err := subCtx.traverseChildren(node); err != nil {
			return err
		}
		order := []atom.Atom{atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6}

		var stars string
		for i, a := range order {
			if node.DataAtom == a {
				stars = strings.Repeat("*", i+1)
			}
		}

		str := strings.ReplaceAll(subCtx.buf.String(), "\n", " ")
		return ctx.emit("\n" + stars + str + "\n")

	case atom.Blockquote:
		ctx.blockquoteLevel++
		if err := ctx.emit("\n"); err != nil {
			return err
		}
		if ctx.blockquoteLevel == 1 {
			if err := ctx.emit("\n#+begin_quote\n"); err != nil {
				return err
			}
		}
		if err := ctx.traverseChildren(node); err != nil {
			return err
		}
		if ctx.blockquoteLevel == 1 {
			if err := ctx.emit("\n#+end_quote\n"); err != nil {
				return err
			}
		}
		ctx.blockquoteLevel--
		return ctx.emit("\n\n")

	case atom.Div:
		if ctx.lineLength > 0 {
			if err := ctx.emit("\n"); err != nil {
				return err
			}
		}
		if err := ctx.traverseChildren(node); err != nil {
			return err
		}
		var err error
		if !ctx.justClosedDiv {
			err = ctx.emit("\n")
		}
		ctx.justClosedDiv = true
		return err

	case atom.Li:
		if err := ctx.emit("- "); err != nil {
			return err
		}

		if err := ctx.traverseChildren(node); err != nil {
			return err
		}

		return ctx.emit("\n")

	case atom.B, atom.Strong:
		subCtx := textifyTraverseContext{
			options: ctx.options,
		}
		subCtx.endsWithSpace = true
		if err := subCtx.traverseChildren(node); err != nil {
			return err
		}
		str := subCtx.buf.String()
		return ctx.emit("*" + str + "*")

	case atom.A:
		linkText := ""
		// For simple link element content with single text node only, peek at the link text.
		if node.FirstChild != nil && node.FirstChild.NextSibling == nil && node.FirstChild.Type == html.TextNode {
			linkText = node.FirstChild.Data
		}

		subCtx := textifyTraverseContext{
			options: ctx.options,
		}

		// If image is the only child, take its alt text as the link text.
		if img := node.FirstChild; img != nil && node.LastChild == img && img.DataAtom == atom.Img {
			if altText := getAttrVal(img, "alt"); altText != "" {
				linkText = altText
				if err := ctx.traverseChildren(node); err != nil {
					return err
				}
			}
		} else {
			if err := subCtx.traverseChildren(node); err != nil {
				return err
			}
			linkText = strings.TrimSpace(subCtx.buf.String())
		}

		hrefLink := ""
		var err error
		if !ctx.options.OmitLinks {
			hrefLink, err = ctx.normalizeHrefLink(strings.TrimSpace(getAttrVal(node, "href")))
			if err != nil {
				return err
			}
		}

		res := ""
		if linkText == "" && hrefLink == "" {
			res = ""
		} else if linkText == hrefLink {
			res = fmt.Sprintf("[[%s]]", linkText)
		} else if linkText != "" && hrefLink != "" {
			res = fmt.Sprintf("[[%s][%s]]", hrefLink, linkText)
		} else if linkText == "" && hrefLink != "" {
			res = fmt.Sprintf("[[%s]]", hrefLink)
		} else if linkText != "" && hrefLink == "" {
			res = fmt.Sprintf("%s", linkText)
		}

		return ctx.emit(res)

	case atom.P, atom.Ul:
		return ctx.paragraphHandler(node)

	case atom.Table, atom.Tfoot, atom.Th, atom.Tr, atom.Td:
		if ctx.options.PrettyTables {
			return ctx.handleTableElement(node)
		} else if node.DataAtom == atom.Table {
			return ctx.paragraphHandler(node)
		}
		return ctx.traverseChildren(node)

	case atom.Img:
		alt := getAttrVal(node, "alt")
		src, err := ctx.normalizeHrefLink(getAttrVal(node, "src"))
		if err != nil {
			return err
		}
		if src == "" {
			return ctx.emit("")
		} else if alt != "" {
			return ctx.emit(fmt.Sprintf(`
#+NAME: %s
[[%s]]
`, alt, src))
		}
		return ctx.emit(fmt.Sprintf("[[%s]]\n", src))

	case atom.Pre:
		ctx.isPre = true
		ctx.emit("\n#+begin_verse\n")
		err := ctx.traverseChildren(node)
		if ctx.endsWithNewLine {
			ctx.emit("#+end_verse\n")
		} else {
			ctx.emit("\n#+end_verse\n")
		}

		ctx.isPre = false
		return err

	case atom.Style, atom.Script, atom.Head:
		// Ignore the subtree.
		return nil

	default:
		return ctx.traverseChildren(node)
	}
}

// paragraphHandler renders node children surrounded by double newlines.
func (ctx *textifyTraverseContext) paragraphHandler(node *html.Node) error {
	if err := ctx.emit("\n\n"); err != nil {
		return err
	}
	if err := ctx.traverseChildren(node); err != nil {
		return err
	}
	return ctx.emit("\n\n")
}

// handleTableElement is only to be invoked when options.PrettyTables is active.
func (ctx *textifyTraverseContext) handleTableElement(node *html.Node) error {
	if !ctx.options.PrettyTables {
		panic("handleTableElement invoked when PrettyTables not active")
	}

	switch node.DataAtom {
	case atom.Table:
		if err := ctx.emit("\n\n"); err != nil {
			return err
		}

		// Re-intialize all table context.
		ctx.tableCtx.init()

		// Browse children, enriching context with table data.
		if err := ctx.traverseChildren(node); err != nil {
			return err
		}

		buf := &bytes.Buffer{}
		table := tablewriter.NewWriter(buf)
		if ctx.options.PrettyTablesOptions != nil {
			options := ctx.options.PrettyTablesOptions
			table.SetAutoFormatHeaders(options.AutoFormatHeader)
			table.SetAutoWrapText(options.AutoWrapText)
			table.SetReflowDuringAutoWrap(options.ReflowDuringAutoWrap)
			table.SetColWidth(options.ColWidth)
			table.SetColumnSeparator(options.ColumnSeparator)
			table.SetRowSeparator(options.RowSeparator)
			table.SetCenterSeparator(options.CenterSeparator)
			table.SetHeaderAlignment(options.HeaderAlignment)
			table.SetFooterAlignment(options.FooterAlignment)
			table.SetAlignment(options.Alignment)
			table.SetColumnAlignment(options.ColumnAlignment)
			table.SetNewLine(options.NewLine)
			table.SetHeaderLine(options.HeaderLine)
			table.SetRowLine(options.RowLine)
			table.SetAutoMergeCells(options.AutoMergeCells)
			table.SetBorders(options.Borders)
		}
		table.SetHeader(ctx.tableCtx.header)
		table.SetFooter(ctx.tableCtx.footer)
		table.AppendBulk(ctx.tableCtx.body)

		// Render the table using ASCII.
		table.Render()
		s := buf.String()

		if ctx.options.PrettyTablesOptions == nil || (ctx.options.PrettyTablesOptions != nil && ctx.options.PrettyTablesOptions.OrgFormat) {
			s = strings.TrimSuffix(s, "\n")
			centerSep := "+"
			if ctx.options.PrettyTablesOptions != nil {
				centerSep = ctx.options.PrettyTablesOptions.CenterSeparator
			}
			firstIndex := strings.Index(s, "\n")
			lastIndex := strings.LastIndex(s, "\n")

			firstLine := s[0:firstIndex]
			lastLine := s[lastIndex:]

			if strings.Contains(lastLine, centerSep) {
				s = s[0:lastIndex]
			}
			if strings.Contains(firstLine, centerSep) {
				s = s[firstIndex:]
			}
		}

		if err := ctx.emit(s); err != nil {
			return err
		}

		return ctx.emit("\n\n")

	case atom.Tfoot:
		ctx.tableCtx.isInFooter = true
		if err := ctx.traverseChildren(node); err != nil {
			return err
		}
		ctx.tableCtx.isInFooter = false

	case atom.Tr:
		ctx.tableCtx.body = append(ctx.tableCtx.body, []string{})
		if err := ctx.traverseChildren(node); err != nil {
			return err
		}
		ctx.tableCtx.tmpRow++

	case atom.Th:
		res, err := ctx.renderEachChild(node)
		if err != nil {
			return err
		}

		ctx.tableCtx.header = append(ctx.tableCtx.header, res)

	case atom.Td:
		res, err := ctx.renderEachChild(node)
		if err != nil {
			return err
		}

		if ctx.tableCtx.isInFooter {
			ctx.tableCtx.footer = append(ctx.tableCtx.footer, res)
		} else {
			ctx.tableCtx.body[ctx.tableCtx.tmpRow] = append(ctx.tableCtx.body[ctx.tableCtx.tmpRow], res)
		}

	}
	return nil
}

func (ctx *textifyTraverseContext) traverse(node *html.Node) error {
	switch node.Type {
	default:
		return ctx.traverseChildren(node)

	case html.TextNode:
		var data string
		if ctx.isPre {
			data = node.Data
		} else {
			data = strings.TrimSpace(spacingRe.ReplaceAllString(node.Data, " "))
		}
		return ctx.emit(data)

	case html.ElementNode:
		return ctx.handleElement(node)
	}
}

func (ctx *textifyTraverseContext) traverseChildren(node *html.Node) error {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if err := ctx.traverse(c); err != nil {
			return err
		}
	}

	return nil
}

func (ctx *textifyTraverseContext) emit(data string) error {
	if data == "" {
		return nil
	}
	var (
		lines = ctx.breakLongLines(data)
		err   error
	)
	for _, line := range lines {
		runes := []rune(line)
		startsWithSpace := unicode.IsSpace(runes[0])
		if !startsWithSpace && !ctx.endsWithSpace && !strings.HasPrefix(data, ".") && !ctx.isPre {
			if err = ctx.buf.WriteByte(' '); err != nil {
				return err
			}
			ctx.lineLength++
		}
		ctx.endsWithSpace = unicode.IsSpace(runes[len(runes)-1])
		ctx.endsWithNewLine = runes[len(runes)-1] == '\n'
		for _, c := range line {
			if _, err = ctx.buf.WriteString(string(c)); err != nil {
				return err
			}
			ctx.lineLength++
			if c == '\n' {
				ctx.lineLength = 0
				if ctx.prefix != "" {
					if _, err = ctx.buf.WriteString(ctx.prefix); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

const maxLineLen = 74

func (ctx *textifyTraverseContext) breakLongLines(data string) []string {
	// Only break lines when in blockquotes.
	if ctx.blockquoteLevel == 0 || !ctx.options.BreakLongLines {
		return []string{data}
	}
	var (
		ret      = []string{}
		runes    = []rune(data)
		l        = len(runes)
		existing = ctx.lineLength
	)
	if existing >= maxLineLen {
		ret = append(ret, "\n")
		existing = 0
	}
	for l+existing > maxLineLen {
		i := maxLineLen - existing
		for i >= 0 && !unicode.IsSpace(runes[i]) {
			i--
		}
		if i == -1 {
			// No spaces, so go the other way.
			i = maxLineLen - existing
			for i < l && !unicode.IsSpace(runes[i]) {
				i++
			}
		}
		ret = append(ret, string(runes[:i])+"\n")
		for i < l && unicode.IsSpace(runes[i]) {
			i++
		}
		runes = runes[i:]
		l = len(runes)
		existing = 0
	}
	if len(runes) > 0 {
		ret = append(ret, string(runes))
	}
	return ret
}

func (ctx *textifyTraverseContext) normalizeHrefLink(link string) (string, error) {
	link = strings.TrimSpace(link)
	link = strings.ReplaceAll(link, "\n", "")
	if ctx.options.BaseURL != "" {
		u, err := url.Parse(link)
		if err != nil {
			return "", err
		}
		base, err := url.Parse(ctx.options.BaseURL)
		if err != nil {
			return "", err
		}
		link = base.ResolveReference(u).String()
	}
	return link, nil
}

// renderEachChild visits each direct child of a node and collects the sequence of
// textuual representaitons separated by a single newline.
func (ctx *textifyTraverseContext) renderEachChild(node *html.Node) (string, error) {
	buf := &bytes.Buffer{}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		s, err := FromHTMLNode(c, ctx.options)
		if err != nil {
			return "", err
		}
		if _, err = buf.WriteString(s); err != nil {
			return "", err
		}
		if c.NextSibling != nil {
			if err = buf.WriteByte('\n'); err != nil {
				return "", err
			}
		}
	}
	return buf.String(), nil
}

func getAttrVal(node *html.Node, attrName string) string {
	for _, attr := range node.Attr {
		if attr.Key == attrName {
			return attr.Val
		}
	}

	return ""
}
