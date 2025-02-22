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

const orgFormIDFormat = "org-form-id--%d"

var allowedInputTypes = map[string]struct{}{
	"text":     {},
	"number":   {},
	"password": {},
	"unknown":  {},
}

// Options provide toggles and overrides to control specific rendering behaviors.
type Options struct {
	PrettyTables        bool                 // Turns on pretty ASCII rendering for table elements.
	PrettyTablesOptions *PrettyTablesOptions // Configures pretty ASCII rendering for table elements.
	OmitLinks           bool                 // Turns on omitting links
	BreakLongLines      bool
	BaseURL             string
	ShowNoscripts       bool
	InternalLinks       bool
	ShowLongDataURL     bool
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
		AutoWrapText:         false,
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
		buf:         bytes.Buffer{},
		fragmentIDs: map[string]struct{}{},
		options:     options,
	}
	ctx.collectFragmentIDs(doc)
	if err := ctx.traverse(doc); err != nil {
		return "", err
	}

	text := ctx.buf.String()
	text = trailingSpaceRe.ReplaceAllString(text, "\n")
	text = newlineRe.ReplaceAllString(text, "\n\n")
	text = normalizeNonBreakingSpace(text)
	text = strings.TrimSpace(text)
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
	spacingRe       = regexp.MustCompile(`[ \r\n\t]+`)
	newlineRe       = regexp.MustCompile(`\n\n+`)
	trailingSpaceRe = regexp.MustCompile(` +\n`)
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
	isPreFormatted  bool
	isInForm        bool
	formCounter     int
	fragmentIDs     map[string]struct{}
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

func (ctx *textifyTraverseContext) traverseWithSubContext(node *html.Node) (textifyTraverseContext, error) {
	subCtx := textifyTraverseContext{
		options:        ctx.options,
		fragmentIDs:    ctx.fragmentIDs,
		isPreFormatted: ctx.isPreFormatted,
		isInForm:       ctx.isInForm,
		formCounter:    ctx.formCounter,
	}
	err := subCtx.traverseChildren(node)
	return subCtx, err
}

func (ctx *textifyTraverseContext) handleElement(node *html.Node) error {
	ctx.justClosedDiv = false

	switch node.DataAtom {
	case atom.Br:
		return ctx.emit("\n")

	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		order := []atom.Atom{atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6}

		var stars string
		for i, a := range order {
			if node.DataAtom == a {
				stars = strings.Repeat("*", i+1)
			}
		}

		subCtx, err := ctx.traverseWithSubContext(node)
		if err != nil {
			return err
		}

		str := strings.TrimSpace(cleanSpacing(subCtx.buf.String()))
		return ctx.emit("\n" + stars + " " + str + "\n")

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
		subCtx, err := ctx.traverseWithSubContext(node)
		if err != nil {
			return err
		}
		s := subCtx.buf.String()
		cleaned := strings.TrimSpace(cleanSpacing(s))
		if cleaned == "" {
			return nil
		}
		ctx.prefix = "- "
		if !ctx.endsWithNewLine {
			ctx.emit("\n")
		}
		ctx.emit(strings.Trim(s, " \n\r\t"))
		ctx.prefix = ""
		return ctx.emit("\n")

	case atom.Dt:
		if !ctx.endsWithNewLine {
			ctx.emit("\n")
		}

		ctx.emit("_")
		if err := ctx.traverseChildren(node); err != nil {
			return err
		}
		return ctx.emit("_\n")

	case atom.Dd:
		if !ctx.endsWithNewLine {
			ctx.emit("\n")
		}

		if err := ctx.traverseChildren(node); err != nil {
			return err
		}
		return ctx.emit("\n")

	case atom.B, atom.Strong:
		subCtx, err := ctx.traverseWithSubContext(node)
		if err != nil {
			return nil
		}
		str := subCtx.buf.String()
		return ctx.emit("*" + str + "*")

	case atom.A:
		linkText := ""
		// For simple link element content with single text node only, peek at the link text.
		if node.FirstChild != nil && node.FirstChild.NextSibling == nil && node.FirstChild.Type == html.TextNode {
			linkText = node.FirstChild.Data
		}

		// If image is the only child, take its alt text as the link text.
		if img := node.FirstChild; img != nil && node.LastChild == img && img.DataAtom == atom.Img {
			if altText := getAttrVal(img, "alt"); altText != "" {
				linkText = altText
				if err := ctx.traverseChildren(node); err != nil {
					return err
				}
			}
		} else if containsBlockLevelAtom(node) {
			linkText = "Link"
			subCtx, err := ctx.traverseWithSubContext(node)
			if err != nil {
				return err
			}
			// make multiline to single line
			s := cleanSpacing(subCtx.buf.String())
			ctx.emit("\n" + strings.TrimPrefix(s, " "))
		} else {
			subCtx, err := ctx.traverseWithSubContext(node)
			if err != nil {
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

		if err := ctx.traverseChildren(node); err != nil {
			return err
		}

		if node.DataAtom == atom.Tr {
			return ctx.emit("\n")
		}

		if node.DataAtom == atom.Td || node.DataAtom == atom.Th {
			return ctx.emit(" ")
		}

		return nil

	case atom.Input:
		t := getAttrVal(node, "type")
		if t == "" {
			t = "unknown"
		}
		value := getAttrVal(node, "value")
		placeholder := getAttrVal(node, "placeholder")
		content := ""
		if value != "" {
			content = value
		} else {
			content = placeholder
		}

		if _, ok := allowedInputTypes[t]; !ok {
			return nil
		}

		if !ctx.isInForm {

			return ctx.emit(fmt.Sprintf(`

#+begin_input _ :type %s
%s
#+end_input

`, t, content))

		} else {
			name := getAttrVal(node, "name")
			id := fmt.Sprintf(orgFormIDFormat, ctx.formCounter)
			return ctx.emit(fmt.Sprintf(`

#+begin_input _ :type %s :id %s :name %s
%s
#+end_input
`, t, id, name, content))
		}

	case atom.Textarea:
		placeholder := getAttrVal(node, "placeholder")
		ctx.isPreFormatted = true
		subCtx, err := ctx.traverseWithSubContext(node)
		ctx.isPreFormatted = false
		if err != nil {
			return err
		}
		content := subCtx.buf.String()
		if content == "" {
			content = placeholder
		}

		if !ctx.isInForm {
			return ctx.emit(fmt.Sprintf(`

#+begin_textarea _
%s
#+end_textarea

`, content))
		} else {
			id := fmt.Sprintf(orgFormIDFormat, ctx.formCounter)
			name := getAttrVal(node, "name")

			return ctx.emit(fmt.Sprintf(`

#+begin_textarea _ :id %s :name %s
%s
#+end_textarea
`, id, name, content))
		}

	case atom.Form:
		method := getAttrVal(node, "method")
		action := getAttrVal(node, "action")
		if method == "" {
			method = "get"
		}
		if action == "" {
			action = ctx.options.BaseURL
		}
		normalized, err := ctx.normalizeHrefLink(action)
		ctx.isInForm = true
		c := ctx.formCounter + 1
		ctx.formCounter = c
		id := fmt.Sprintf(orgFormIDFormat, c)
		link := fmt.Sprintf("[[org-form:%s:%s:%s][Submit]]\n\n", id, method, normalized)
		if err != nil {
			return err
		}
		err = ctx.traverseChildren(node)
		ctx.emit(link)
		ctx.isInForm = false
		return err

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
#+CAPTION: %s
[[%s]]
`, alt, src))
		}
		return ctx.emit(fmt.Sprintf("[[%s]]\n", src))

	case atom.Pre:
		if ctx.isPreFormatted {
			return ctx.traverseChildren(node)
		}

		ctx.isPreFormatted = true
		ctx.emit("\n#+begin_src\n")
		err := ctx.traverseChildren(node)
		if !ctx.endsWithNewLine {
			ctx.emit("\n")
		}
		ctx.emit("#+end_src\n")

		ctx.isPreFormatted = false
		return err

	case atom.Samp, atom.Kbd, atom.Tt, atom.Var, atom.Code:
		if ctx.isPreFormatted {
			return ctx.traverseChildren(node)
		}

		subCtx, err := ctx.traverseWithSubContext(node)
		if err != nil {
			return err
		}

		result := strings.TrimSpace(subCtx.buf.String())
		if strings.Contains(result, "\n") {
			ctx.emit(fmt.Sprintf("\n#+begin_src\n%s\n#+end_src\n", result))
		} else {
			ctx.emit(fmt.Sprintf("~%s~", result))
		}

		return nil

	case atom.Title:
		ctx.emit("#+TITLE: ")
		err := ctx.traverseChildren(node)
		if err != nil {
			return nil
		}
		return ctx.emit("\n\n\n")

	case atom.Noscript:
		if ctx.options.ShowNoscripts && node.FirstChild != nil {
			s, err := FromString(node.FirstChild.Data)
			if err != nil {
				return err
			}
			ctx.emit(s)
		}
		return nil

	case atom.Style, atom.Script, atom.Meta, atom.Link:
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
		var options *PrettyTablesOptions
		if ctx.options.PrettyTablesOptions != nil {
			options = ctx.options.PrettyTablesOptions
		} else {
			options = NewPrettyTablesOptions()
		}
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

		table.SetHeader(ctx.tableCtx.header)
		table.SetFooter(ctx.tableCtx.footer)
		table.AppendBulk(ctx.tableCtx.body)

		// Render the table using ASCII.
		table.Render()
		s := buf.String()

		if options.OrgFormat {
			s = strings.TrimSuffix(s, "\n")

			// remove top, bottom boarders
			// if options.Borders are used, footer format is invalid as org.
			// thus delete here
			centerSep := options.CenterSeparator
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

			// change center sep with ColumnSeparator on the left/right borders
			s = strings.ReplaceAll(s, "\n+", "\n"+options.ColumnSeparator)
			s = strings.ReplaceAll(s, "+\n", options.ColumnSeparator+"\n")
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

func (ctx *textifyTraverseContext) handleInternalLinks(node *html.Node) error {
	if !ctx.options.InternalLinks {
		return nil
	}
	id := getAttrVal(node, "id")
	name := getAttrVal(node, "name")
	_, matchedID := ctx.fragmentIDs[id]
	_, matchedName := ctx.fragmentIDs[name]
	if matchedID || matchedName {
		var frag string
		if matchedID {
			frag = id
		} else {
			frag = name
		}
		endsWithNewLine := ctx.endsWithNewLine
		if endsWithNewLine {
			b := ctx.buf.Bytes()
			ctx.buf = *bytes.NewBuffer(b[0 : len(b)-1])
			ctx.endsWithNewLine = false
		}
		if err := ctx.emit(" <<" + frag + ">> "); err != nil {
			return err
		}
		if endsWithNewLine {
			ctx.emit("\n")
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
		if ctx.isPreFormatted {
			data = node.Data
		} else {
			data = cleanSpacing(node.Data)
		}
		return ctx.emit(data)

	case html.ElementNode:
		if err := ctx.handleElement(node); err != nil {
			return err
		}

		return ctx.handleInternalLinks(node)
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
		// lines = strings.Split(data, "\n") TODO
		err error
	)
	for _, line := range lines {
		runes := []rune(line)

		if !ctx.isPreFormatted && ctx.endsWithNewLine {
			line = strings.TrimPrefix(line, " ")
			if ctx.prefix != "" {
				ctx.endsWithNewLine = false
				if _, err = ctx.buf.WriteString(ctx.prefix); err != nil {
					return err
				}
			}
		}

		if line != "" {
			ctx.endsWithNewLine = runes[len(runes)-1] == '\n'
		}

		for _, c := range line {
			if _, err = ctx.buf.WriteString(string(c)); err != nil {
				return err
			}
			ctx.lineLength++
			if c == '\n' {
				ctx.lineLength = 0
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
	if link == "" {
		return link, nil
	}
	if strings.HasPrefix(link, "#") {
		return link[1:], nil
	}
	if !ctx.options.ShowLongDataURL && strings.HasPrefix(link, "data:") && len(link) > 100 {
		splitted := strings.Split(link, ";")
		if len(splitted) > 0 {
			return splitted[0] + ";(omitted)", nil
		}
		return "data:(omitted)", nil
	}

	link = strings.TrimSpace(link)
	link = strings.ReplaceAll(link, "\n", "")
	if ctx.options.BaseURL != "" {
		u, err := url.Parse(link)
		if err != nil {
			s := err.Error()
			// try to ignore invalid part and use vaid part
			// parse "ssssss#%_tbar": invalid URL escape "%_t"
			message := "invalid URL escape "
			if !strings.Contains(s, message) {
				return "", err
			}
			splitted := strings.Split(s, message)
			invalid := strings.Trim(splitted[len(splitted)-1], `"`)
			validPart := strings.Split(link, invalid)[0]
			u, err = url.Parse(validPart)
			if err != nil {
				return "", err
			}
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

		if _, isBlockLevel := blockLevelAtoms[c.DataAtom]; c.NextSibling != nil && isBlockLevel {
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

var blockLevelAtoms = map[atom.Atom]struct{}{
	atom.Address:    {},
	atom.Article:    {},
	atom.Aside:      {},
	atom.Blockquote: {},
	atom.Canvas:     {},
	atom.Dd:         {},
	atom.Div:        {},
	atom.Dl:         {},
	atom.Dt:         {},
	atom.Fieldset:   {},
	atom.Figcaption: {},
	atom.Figure:     {},
	atom.Footer:     {},
	atom.Form:       {},
	atom.H1:         {},
	atom.H2:         {},
	atom.H3:         {},
	atom.H4:         {},
	atom.H5:         {},
	atom.H6:         {},
	atom.Header:     {},
	atom.Hr:         {},
	atom.Li:         {},
	atom.Main:       {},
	atom.Nav:        {},
	atom.Noscript:   {},
	atom.Ol:         {},
	atom.P:          {},
	atom.Pre:        {},
	atom.Section:    {},
	atom.Table:      {},
	atom.Tfoot:      {},
	atom.Ul:         {},
	atom.Video:      {},
}

func containsBlockLevelAtom(node *html.Node) bool {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		_, ok := blockLevelAtoms[c.DataAtom]
		if ok {
			return true
		}
	}
	return false
}

func cleanSpacing(s string) string {
	s = spacingRe.ReplaceAllString(s, " ")
	lastIsSpace := false
	buf := bytes.Buffer{}
	for _, c := range s {
		spaceRepeated := lastIsSpace && c == ' '
		if !spaceRepeated {
			buf.WriteRune(c)
		}

		if c == ' ' {
			lastIsSpace = true
		} else {
			lastIsSpace = false
		}
	}
	return buf.String()
}

func normalizeNonBreakingSpace(s string) string {
	buf := bytes.Buffer{}
	for _, c := range s {
		// non-breaking space
		if c == 160 {
			buf.WriteRune(' ')
		} else {
			buf.WriteRune(c)
		}
	}
	return buf.String()
}

func (ctx *textifyTraverseContext) collectFragmentIDs(node *html.Node) {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if c.DataAtom == atom.A {
			href := getAttrVal(c, "href")
			if strings.HasPrefix(href, "#") && len(href) > 1 {
				ctx.fragmentIDs[href[1:]] = struct{}{}
			}
		}
		ctx.collectFragmentIDs(c)
	}
}
