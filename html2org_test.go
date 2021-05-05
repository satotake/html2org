package html2org

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
)

const destPath = "testdata"

// EnableExtraLogging turns on additional testing log output.
// Extra test logging can be enabled by setting the environment variable
// HTML2TEXT_EXTRA_LOGGING to "1" or "true".
var EnableExtraLogging bool

func init() {
	if v := os.Getenv("HTML2TEXT_EXTRA_LOGGING"); v == "1" || v == "true" {
		EnableExtraLogging = true
	}
}

// TODO Add tests for FromHTMLNode and FromReader.

func TestParseUTF8(t *testing.T) {
	htmlFiles := []struct {
		file                  string
		keywordShouldNotExist string
		keywordShouldExist    string
	}{
		{
			"utf8.html",
			"学习之道:美国公认学习第一书script",
			"次世界冠军赛上，我几近疯狂",
		},
		{
			"utf8_with_bom.xhtml",
			"1892年波兰文版序言script",
			"种新的波兰文本已成为必要",
		},
	}

	for _, htmlFile := range htmlFiles {
		bs, err := ioutil.ReadFile(path.Join(destPath, htmlFile.file))
		if err != nil {
			t.Fatal(err)
		}
		text, err := FromReader(bytes.NewReader(bs))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, htmlFile.keywordShouldExist) {
			t.Fatalf("keyword %s should  exists in file %s", htmlFile.keywordShouldExist, htmlFile.file)
		}
		if strings.Contains(text, htmlFile.keywordShouldNotExist) {
			t.Fatalf("keyword %s should not exists in file %s", htmlFile.keywordShouldNotExist, htmlFile.file)
		}
	}
}

func TestStrippingWhitespace(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"test text",
			"test text",
		},
		{
			"  \ttext\ntext\n",
			"text text",
		},
		{
			"  \na \n\t \n \n a \t",
			"a a",
		},
		{
			"test        text",
			"test text",
		},
		{
			"test&nbsp;&nbsp;&nbsp; text&nbsp;",
			"test    text",
			// normal whitespace:30
		},
		{
			`<p>This is &lsquo;<span>foo</span>&rsquo;.</p>`,
			`This is ‘foo’.`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestParagraphsAndBreaks(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"Test text",
			"Test text",
		},
		{
			"Test text<br>",
			"Test text",
		},
		{
			"Test text<br>Test",
			"Test text\nTest",
		},
		{
			"<p>Test text</p>",
			"Test text",
		},
		{
			"<p>Test text</p><p>Test text</p>",
			"Test text\n\nTest text",
		},
		{
			"\n<p>Test text</p>\n\n\n\t<p>Test text</p>\n",
			"Test text\n\nTest text",
		},
		{
			"\n<p>Test text<br/>Test text</p>\n",
			"Test text\nTest text",
		},
		{
			"\n<p>Test text<br> \tTest text<br></p>\n",
			"Test text\nTest text",
		},
		{
			"Test text<br><BR />Test text",
			"Test text\n\nTest text",
		},
		{
			"<pre>test1\ntest 2\n\ntest  3\n</pre>",
			`#+begin_src
test1
test 2

test  3
#+end_src`,
		},
		{
			"<pre>test 1   test 2</pre>",
			`#+begin_src
test 1   test 2
#+end_src`,
		},
		{
			`<pre class="chroma">
    <span class="nx">b1</span> <span class="o">:=</span> <span class="nb">make</span><span class="p">([]</span><span class="kt">byte</span><span class="p">,</span> <span class="mi">5</span><span class="p">)</span>
</pre>`,
			`#+begin_src
    b1 := make([]byte, 5)
#+end_src`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestCodeRelatedTags(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		// single line
		{
			`<p>The first <tt class="key">KEY</tt> part.`,
			`The first ~KEY~ part.`,
		},
		{
			`<p>This is <kbd>kbd</kbd>.`,
			`This is ~kbd~.`,
		},
		{
			`<p>This is <var>var</var>.`,
			`This is ~var~.`,
		},
		{
			`<p>the argument to <code>code</code> is</p>`,
			`the argument to ~code~ is`,
		},
		{
			`<p>This is <samp>samp</samp>.</p>`,
			`This is ~samp~.`,
		},
		// multiple line
		{
			`<p>Multi-line<tt class="key">teletype<br>TELETYPE</tt> part.`,
			`Multi-line
#+begin_src
teletype
TELETYPE
#+end_src
part.`,
		},
		{
			`<pre><code>a := 1
b := 2
</code></pre>
`,
			`#+begin_src
a := 1
b := 2
#+end_src`,
		},
		{
			`<pre><code>func foo()  {
    return 1
}
</code></pre>
`,
			`#+begin_src
func foo()  {
    return 1
}
#+end_src`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestTables(t *testing.T) {
	testCases := []struct {
		input           string
		tabularOutput   string
		plaintextOutput string
	}{
		{
			"<table><tr><td></td><td></td></tr></table>",
			// Empty table
			// |  |  |
			"|  |  |",
			"",
		},
		{
			"<table><tr><td>cell1</td><td>cell2</td></tr></table>",
			// | cell1 | cell2 |
			"| cell1 | cell2 |",
			"cell1 cell2",
		},
		{
			"<table><tr><td>row1</td></tr><tr><td>row2</td></tr></table>",
			// | row1 |
			// | row2 |
			"| row1 |\n| row2 |",
			"row1\nrow2",
		},
		{
			`<table>
				<tbody>
					<tr><td><p>Row-1-Col-1-Msg123456789012345</p><p>Row-1-Col-1-Msg2</p></td><td>Row-1-Col-2</td></tr>
					<tr><td>Row-2-Col-1</td><td>Row-2-Col-2</td></tr>
				</tbody>
			</table>`,
			// | Row-1-Col-1-Msg123456789012345 | Row-1-Col-2 |
			// | Row-1-Col-1-Msg2               |             |
			// | Row-2-Col-1                    | Row-2-Col-2 |
			`| Row-1-Col-1-Msg123456789012345 | Row-1-Col-2 |
| Row-1-Col-1-Msg2               |             |
| Row-2-Col-1                    | Row-2-Col-2 |`,
			`Row-1-Col-1-Msg123456789012345

Row-1-Col-1-Msg2

Row-1-Col-2
Row-2-Col-1 Row-2-Col-2`,
		},
		{
			`<table>
			   <tr><td>cell1-1</td><td>cell1-2</td></tr>
			   <tr><td>cell2-1</td><td>cell2-2</td></tr>
			</table>`,
			// | cell1-1 | cell1-2 |
			// | cell2-1 | cell2-2 |
			"| cell1-1 | cell1-2 |\n| cell2-1 | cell2-2 |",
			"cell1-1 cell1-2\ncell2-1 cell2-2",
		},
		{
			`<table>
				<thead>
					<tr><th>Header 1</th><th>Header 2</th></tr>
				</thead>
				<tfoot>
					<tr><td>Footer 1</td><td>Footer 2</td></tr>
				</tfoot>
				<tbody>
					<tr><td>Row 1 Col 1</td><td>Row 1 Col 2</td></tr>
					<tr><td>Row 2 Col 1</td><td>Row 2 Col 2</td></tr>
				</tbody>
			</table>`,
			`|  HEADER 1   |  HEADER 2   |
|-------------+-------------|
| Row 1 Col 1 | Row 1 Col 2 |
| Row 2 Col 1 | Row 2 Col 2 |
|-------------+-------------|
|  FOOTER 1   |  FOOTER 2   |`,
			`Header 1 Header 2
Footer 1 Footer 2
Row 1 Col 1 Row 1 Col 2
Row 2 Col 1 Row 2 Col 2`,
		},
		// Two tables in same HTML (goal is to test that context is
		// reinitialized correctly).
		{
			`<p>
				<table>
					<thead>
						<tr><th>Table 1 Header 1</th><th>Table 1 Header 2</th></tr>
					</thead>
					<tfoot>
						<tr><td>Table 1 Footer 1</td><td>Table 1 Footer 2</td></tr>
					</tfoot>
					<tbody>
						<tr><td>Table 1 Row 1 Col 1</td><td>Table 1 Row 1 Col 2</td></tr>
						<tr><td>Table 1 Row 2 Col 1</td><td>Table 1 Row 2 Col 2</td></tr>
					</tbody>
				</table>
				<table>
					<thead>
						<tr><th>Table 2 Header 1</th><th>Table 2 Header 2</th></tr>
					</thead>
					<tfoot>
						<tr><td>Table 2 Footer 1</td><td>Table 2 Footer 2</td></tr>
					</tfoot>
					<tbody>
						<tr><td>Table 2 Row 1 Col 1</td><td>Table 2 Row 1 Col 2</td></tr>
						<tr><td>Table 2 Row 2 Col 1</td><td>Table 2 Row 2 Col 2</td></tr>
					</tbody>
				</table>
			</p>`,
			`|  TABLE 1 HEADER 1   |  TABLE 1 HEADER 2   |
|---------------------+---------------------|
| Table 1 Row 1 Col 1 | Table 1 Row 1 Col 2 |
| Table 1 Row 2 Col 1 | Table 1 Row 2 Col 2 |
|---------------------+---------------------|
|  TABLE 1 FOOTER 1   |  TABLE 1 FOOTER 2   |

|  TABLE 2 HEADER 1   |  TABLE 2 HEADER 2   |
|---------------------+---------------------|
| Table 2 Row 1 Col 1 | Table 2 Row 1 Col 2 |
| Table 2 Row 2 Col 1 | Table 2 Row 2 Col 2 |
|---------------------+---------------------|
|  TABLE 2 FOOTER 1   |  TABLE 2 FOOTER 2   |`,
			`Table 1 Header 1 Table 1 Header 2
Table 1 Footer 1 Table 1 Footer 2
Table 1 Row 1 Col 1 Table 1 Row 1 Col 2
Table 1 Row 2 Col 1 Table 1 Row 2 Col 2

Table 2 Header 1 Table 2 Header 2
Table 2 Footer 1 Table 2 Footer 2
Table 2 Row 1 Col 1 Table 2 Row 1 Col 2
Table 2 Row 2 Col 1 Table 2 Row 2 Col 2`,
		},
		{
			"_<table><tr><td>cell</td></tr></table>_",
			"_\n\n| cell |\n\n_",
			"_\n\ncell\n\n_",
		},
		{
			`<table>
				<tr>
					<th>Item</th>
					<th>Description</th>
					<th>Price</th>
				</tr>
				<tr>
					<td>Golang</td>
					<td>Open source programming language that makes it easy to build simple, reliable, and efficient software</td>
					<td>$10.99</td>
				</tr>
				<tr>
					<td>Hermes</td>
					<td>Programmatically create beautiful e-mails using Golang.</td>
					<td>$1.99</td>
				</tr>
			</table>`,
			`|  ITEM  |                                              DESCRIPTION                                              | PRICE  |
|--------+-------------------------------------------------------------------------------------------------------+--------|
| Golang | Open source programming language that makes it easy to build simple, reliable, and efficient software | $10.99 |
| Hermes | Programmatically create beautiful e-mails using Golang.                                               | $1.99  |`,
			`Item  Description  Price
Golang  Open source programming language that makes it easy to build simple, reliable, and efficient software  $10.99
Hermes  Programmatically create beautiful e-mails using Golang.  $1.99`,
		},
		{
			`<table><tr>
 <td><span></span></td>
 <td><span></span>1</td>
 <td><a href="http://example.com/2"><span></span>2</a></td>
 <td><a href="http://example.com/3"><span></span>3</a></td>
</tr></table>`,
			`|  | 1 | [[http://example.com/2][2]] | [[http://example.com/3][3]] |`,
			`1  [[http://example.com/2][2]]  [[http://example.com/3][3]]`,
		},
	}

	for _, testCase := range testCases {
		options := Options{
			PrettyTables:        true,
			PrettyTablesOptions: NewPrettyTablesOptions(),
		}
		// Check pretty tabular ASCII version.
		if msg, err := wantString(testCase.input, testCase.tabularOutput, options); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}

		// Check plain version.
		if msg, err := wantString(testCase.input, testCase.plaintextOutput); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestStrippingLists(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"<ul></ul>",
			"",
		},
		{
			"<ul><li>item</li></ul>_",
			"- item\n\n_",
		},
		{
			"<li class='123'>item 1</li> <li>item 2</li>\n_",
			"- item 1\n- item 2\n_",
		},
		{
			"<li>item 1</li> \t\n <li>item 2</li> <li> item 3</li>\n_",
			"- item 1\n- item 2\n- item 3\n_",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestNoscripts(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			`<body><noscript><style>div {display: none}</style> <div>Test</div></noscript></body>`,
			`Test`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output, Options{ShowNoscripts: true}); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestTitles(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			`<title>My site</title>text`,
			`#+TITLE: My site

text`,
		},
		{
			`<html><head><title>My site</title></head><body>body</body></html>`,
			`#+TITLE: My site

body`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output, Options{}); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestTextAreas(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			`<textarea></textarea>`,
			`#+begin_textarea

#+end_textarea`,
		},
		{
			`<textarea placeholder="Enter..."></textarea>`,
			`#+begin_textarea
Enter...
#+end_textarea`,
		},
		{
			`<textarea placeholder="Enter...">Entered</textarea>`,
			`#+begin_textarea
Entered
#+end_textarea`,
		},
		{
			`<textarea>func foo() {
    return 1
}</textarea>`,
			`#+begin_textarea
func foo() {
    return 1
}
#+end_textarea`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output, Options{ShowNoscripts: true}); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestInputs(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			`<input>`,
			`#+begin_input :type unknown

#+end_input`,
		},
		{
			`<input />`,
			`#+begin_input :type unknown

#+end_input`,
		},
		{
			`<input type="text" >`,
			`#+begin_input :type text

#+end_input`,
		},
		{
			`<input type="number" >`,
			`#+begin_input :type number

#+end_input`,
		},
		{
			`<input type="password" >`,
			`#+begin_input :type password

#+end_input`,
		},
		// TODO other types
		{
			`<input type="radio" >`,
			``,
		},
		{
			`<input type="hidden" >`,
			``,
		},
		{
			`<input placeholder="Enter input" />`,
			`#+begin_input :type unknown
Enter input
#+end_input`,
		},
		// readonly is ignored
		{
			`<input type="text" value="git@github.com:satotake/html2org.git" readonly="">`,
			`#+begin_input :type text
git@github.com:satotake/html2org.git
#+end_input`,
		},
		// use not placeholder but value for display
		{
			`<input type="text" value="VALUE" placeholder="P">`,
			`#+begin_input :type text
VALUE
#+end_input`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output, Options{ShowNoscripts: true}); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestBaseURLOption(t *testing.T) {
	testCases := []struct {
		baseURL string
		input   string
		output  string
	}{
		{
			"https://orgmode.org/manual/Structure-Templates.html",
			`<a name="index-template-insertion"></a>`,
			``,
		},
		{
			"http://example.com/foo/",
			`<a href="./bar/">bar</a>`,
			`[[http://example.com/foo/bar/][bar]]`,
		},
		{
			"http://example.com/foo/",
			`<a href="../">top</a>`,
			`[[http://example.com/][top]]`,
		},
		{
			"http://example.com/foo/",
			`<img src="hello.jpg">`,
			`[[http://example.com/foo/hello.jpg]]`,
		},
		{
			"http://example.com",
			`<h2><a href="/foo/bar/"><div><span>Title</span> <span>Sub</span></div></a></h2>`,
			`** Title Sub [[http://example.com/foo/bar/][Link]]`,
		},
		{
			"https://mitpress.mit.edu",
			`<a href="book-Z-H-4.html#%_toc_start">content</a>`,
			"[[https://mitpress.mit.edu/book-Z-H-4.html][content]]",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output, Options{BaseURL: testCase.baseURL}); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestLinks(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			`<a></a>`,
			``,
		},
		{
			`<a href=""></a>`,
			``,
		},
		{
			`<a href="http://example.com/"></a>`,
			`[[http://example.com/]]`,
		},
		{
			`<a href="">Link</a>`,
			`Link`,
		},
		{
			`<a href="http://example.com/">Link</a>`,
			`[[http://example.com/][Link]]`,
		},
		{
			`<a href="http://example.com/"><span class="a">Link</span></a>`,
			`[[http://example.com/][Link]]`,
		},
		{
			"<a href='http://example.com/'>\n\t<span class='a'>Link</span>\n\t</a>",
			`[[http://example.com/][Link]]`,
		},
		{
			"<a href='mailto:contact@example.org'>Contact Us</a>",
			`[[mailto:contact@example.org][Contact Us]]`,
		},
		{
			"<a href=\"http://example.com:80/~user?aaa=bb&amp;c=d,e,f#foo\">Link</a>",
			`[[http://example.com:80/~user?aaa=bb&c=d,e,f#foo][Link]]`,
		},
		{
			"<a title='title' href=\"http://example.com/\">Link</a>",
			`[[http://example.com/][Link]]`,
		},
		{
			"<a href=\"   http://example.com/ \"> Link </a>",
			`[[http://example.com/][Link]]`,
		},
		{
			"<a href=\"http://example.com/a/\">Link A</a> <a href=\"http://example.com/b/\">Link B</a>",
			`[[http://example.com/a/][Link A]] [[http://example.com/b/][Link B]]`,
		},
		{
			"<a href=\"%%LINK%%\">Link</a>",
			`[[%%LINK%%][Link]]`,
		},
		{
			"<a href=\"[LINK]\">Link</a>",
			`[[[LINK]][Link]]`,
		},
		{
			"<a href=\"{LINK}\">Link</a>",
			`[[{LINK}][Link]]`,
		},
		{
			"<a href=\"[[!unsubscribe]]\">Link</a>",
			`[[[[!unsubscribe]]][Link]]`,
		},
		{
			"<p>This is <a href=\"http://www.google.com\" >link1</a> and <a href=\"http://www.google.com\" >link2 </a> is next.</p>",
			`This is [[http://www.google.com][link1]] and [[http://www.google.com][link2]] is next.`,
		},
		{
			"<a href=\"http://www.google.com\" >http://www.google.com</a>",
			`[[http://www.google.com]]`,
		},
		{
			`text <a href="http://example.com"><br><h3>Heading</h3><div></div></a>`,
			"text\n*** Heading [[http://example.com][Link]]",
		},
		{
			`<p>(see <a href="http://example.com">Plain Lists</a>)</p>`,
			`(see [[http://example.com][Plain Lists]])`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestOmitLinks(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			`<a></a>`,
			``,
		},
		{
			`<a href=""></a>`,
			``,
		},
		{
			`<a href="http://example.com/"></a>`,
			``,
		},
		{
			`<a href="">Link</a>`,
			`Link`,
		},
		{
			`<a href="http://example.com/">Link</a>`,
			`Link`,
		},
		{
			`<a href="http://example.com/"><span class="a">Link</span></a>`,
			`Link`,
		},
		{
			"<a href='http://example.com/'>\n\t<span class='a'>Link</span>\n\t</a>",
			`Link`,
		},
		{
			`<a href="http://example.com/"><img src="http://example.ru/hello.jpg" alt="Example"></a>`,
			`#+NAME: Example
[[http://example.ru/hello.jpg]]
Example`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output, Options{OmitLinks: true}); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestImageAltTags(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			`<img />`,
			``,
		},
		{
			`<img src="http://example.ru/hello.jpg" />`,
			`[[http://example.ru/hello.jpg]]`,
		},
		{
			`<img alt="Example"/>`,
			``,
		},
		{
			`<img src="http://example.ru/hello.jpg" alt="Example"/>`,
			`#+NAME: Example
[[http://example.ru/hello.jpg]]`,
		},
		// Images do matter if they are in a link.
		{
			`<a href="http://example.com/"><img src="http://example.ru/hello.jpg" alt="Example"/></a>`,
			`#+NAME: Example
[[http://example.ru/hello.jpg]]
[[http://example.com/][Example]]`,
		},
		{
			`<a href='http://example.com/'><img src='http://example.ru/hello.jpg' alt='Example'></a>`,
			`#+NAME: Example
[[http://example.ru/hello.jpg]]
[[http://example.com/][Example]]`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestHeadings(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"<h1>Test</h1>",
			"* Test",
		},
		{
			"\t<h1>\nTest</h1> ",
			"* Test",
		},
		{
			"\t<h1>\nTest line 1<br>Test 2</h1> ",
			"* Test line 1 Test 2",
		},
		{
			"<h1>Test</h1> <h1>Test</h1>",
			"* Test\n\n* Test",
		},
		{
			"<h2>Test</h2>",
			"** Test",
		},
		{
			"<h1><a href='http://example.com/'>Test</a></h1>",
			"* [[http://example.com/][Test]]",
		},
		{
			"<h3> <span class='a'>Test </span></h3>",
			"*** Test",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}

}

func TestBold(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"<b>Test</b>",
			"*Test*",
		},
		{
			"\t<b>Test</b> ",
			"*Test*",
		},
		{
			"\t<b>Test line 1<br>Test 2</b> ",
			"*Test line 1\nTest 2*",
		},
		{
			"<b>Test</b> <b>Test</b>",
			"*Test* *Test*",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}

}

func TestDiv(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"<div>Test</div>",
			"Test",
		},
		{
			"\t<div>Test</div> ",
			"Test",
		},
		{
			"<div>Test line 1<div>Test 2</div></div>",
			"Test line 1\nTest 2",
		},
		{
			"Test 1<div>Test 2</div> <div>Test 3</div>Test 4",
			"Test 1\nTest 2\nTest 3\nTest 4",
		},
		{
			"Test 1<div>&nbsp;Test 2&nbsp;</div>",
			"Test 1\n Test 2",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}

}

func TestBlockquotes(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"<div>level 0<blockquote>level 1<br><blockquote>level 2</blockquote>level 1</blockquote><div>level 0</div></div>",
			`level 0

#+begin_quote
level 1

level 2

level 1
#+end_quote

level 0`,
		},
		{
			"<blockquote>Test</blockquote>Test",
			`#+begin_quote
Test
#+end_quote

Test`,
		},
		{
			"\t<blockquote> \nTest<br></blockquote> ",
			`#+begin_quote
Test

#+end_quote`,
		},
		{
			"\t<blockquote> \nTest line 1<br>Test 2</blockquote> ",
			`#+begin_quote
Test line 1
Test 2
#+end_quote`,
		},
		{
			"<blockquote>Test</blockquote> <blockquote>Test</blockquote> Other Test",
			`#+begin_quote
Test
#+end_quote

#+begin_quote
Test
#+end_quote

Other Test`,
		},
		{
			"<blockquote>Lorem ipsum Commodo id consectetur pariatur ea occaecat minim aliqua ad sit consequat quis ex commodo Duis incididunt eu mollit consectetur fugiat voluptate dolore in pariatur in commodo occaecat Ut occaecat velit esse labore aute quis commodo non sit dolore officia Excepteur cillum amet cupidatat culpa velit labore ullamco dolore mollit elit in aliqua dolor irure do</blockquote>",
			`#+begin_quote
Lorem ipsum Commodo id consectetur pariatur ea occaecat minim aliqua ad sit consequat quis ex commodo Duis incididunt eu mollit consectetur fugiat voluptate dolore in pariatur in commodo occaecat Ut occaecat velit esse labore aute quis commodo non sit dolore officia Excepteur cillum amet cupidatat culpa velit labore ullamco dolore mollit elit in aliqua dolor irure do
#+end_quote`,
		},
		{
			"<blockquote>Lorem <b>ipsum</b> <b>Commodo</b>.</blockquote>",
			`#+begin_quote
Lorem *ipsum* *Commodo*.
#+end_quote`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}

}

func TestIgnoreStylesScriptsHead(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"<style>Test</style>",
			"",
		},
		{
			"<style type=\"text/css\">body { color: #fff; }</style>",
			"",
		},
		{
			"<link rel=\"stylesheet\" href=\"main.css\">",
			"",
		},
		{
			"<script>Test</script>",
			"",
		},
		{
			"<script src=\"main.js\"></script>",
			"",
		},
		{
			"<script type=\"text/javascript\" src=\"main.js\"></script>",
			"",
		},
		{
			"<script type=\"text/javascript\">Test</script>",
			"",
		},
		{
			"<script type=\"text/ng-template\" id=\"template.html\"><a href=\"http://google.com\">Google</a></script>",
			"",
		},
		{
			"<script type=\"bla-bla-bla\" id=\"template.html\">Test</script>",
			"",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestText(t *testing.T) {
	testCases := []struct {
		input string
		expr  string
	}{
		{
			`<li>
		  <a href="/new" data-ga-click="Header, create new repository, icon:repo"><span class="octicon octicon-repo"></span> New repository</a>
		</li>`,
			"- [[/new][New repository]]",
		},
		{
			`hi

			<br>

	hello <a href="https://google.com">google</a>
	<br><br>
	test<p>List:</p>

	<ul>
		<li><a href="foo">Foo</a></li>
		<li><a href="http://www.microshwhat.com/bar/soapy">Barsoap</a></li>
        <li>Baz</li>
	</ul>
`,
			`hi
hello [[https://google.com][google]]

test

List:

- [[foo][Foo]]
- [[http://www.microshwhat.com/bar/soapy][Barsoap]]
- Baz`,
		},
		// Malformed input html.
		{
			`hi

			hello <a href="https://google.com">google</a>

			test<p>List:</p>

			<ul>
				<li><a href="foo">Foo</a>
				<li><a href="/
		                bar/baz">Bar</a>
		        <li>Baz</li>
			</ul>
		`,
			`hi hello [[https://google.com][google]] test

List:

- [[foo][Foo]]
- [[/		                bar/baz][Bar]]
- Baz`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.expr); err != nil {
			// if msg, err := wantRegExp(testCase.input, testCase.expr); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestPeriod(t *testing.T) {
	testCases := []struct {
		input string
		expr  string
	}{
		{
			`<p>Lorem ipsum <span>test</span>.</p>`,
			`Lorem ipsum test\.`,
		},
		{
			`<p>Lorem ipsum <span>test.</span></p>`,
			`Lorem ipsum test\.`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantRegExp(testCase.input, testCase.expr); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

type StringMatcher interface {
	MatchString(string) bool
	String() string
}

type RegexpStringMatcher string

func (m RegexpStringMatcher) MatchString(str string) bool {
	return regexp.MustCompile(string(m)).MatchString(str)
}
func (m RegexpStringMatcher) String() string {
	return string(m)
}

type ExactStringMatcher string

func (m ExactStringMatcher) MatchString(str string) bool {
	return string(m) == str
}
func (m ExactStringMatcher) String() string {
	return string(m)
}

func wantRegExp(input string, outputRE string, options ...Options) (string, error) {
	return match(input, RegexpStringMatcher(outputRE), options...)
}

func wantString(input string, output string, options ...Options) (string, error) {
	return match(input, ExactStringMatcher(output), options...)
}

func match(input string, matcher StringMatcher, options ...Options) (string, error) {
	text, err := FromString(input, options...)
	if err != nil {
		return "", err
	}
	if !matcher.MatchString(text) {
		return "", fmt.Errorf(`error: input did not match specified expression
Input:
>>>>
%v
<<<<

Output:
>>>>
%v
<<<<

Expected:
>>>>
%v
<<<<`,
			input,
			text,
			matcher.String(),
		)
	}

	var msg string

	if EnableExtraLogging {
		msg = fmt.Sprintf(
			`
input:

%v

output:

%v
`,
			input,
			text,
		)
	}
	return msg, nil
}

func TestCleanSpacing(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"\n foo bar  baz",
			" foo bar baz",
		},
		{
			"\n  \n\nfoo\nbar\nbaz",
			" foo bar baz",
		},
		{
			"foo\nbar\nbaz\n\n",
			"foo bar baz ",
		},
	}

	for _, testCase := range testCases {
		got := cleanSpacing(testCase.input)
		want := testCase.output
		if got != want {
			t.Errorf("\ngot : %q\nwant: %q", got, want)
		}
	}
}

func Example() {
	inputHTML := `
<html>
	<head>
		<title>My Mega Service</title>
		<link rel=\"stylesheet\" href=\"main.css\">
		<style type=\"text/css\">body { color: #fff; }</style>
	</head>

	<body>
		<div class="logo">
			<a href="http://jaytaylor.com/"><img src="/logo-image.jpg" alt="Mega Service"/></a>
		</div>

		<h1>Welcome to your new account on my service!</h1>

		<p>
			Here is some more information:

			<ul>
				<li>Link 1: <a href="https://example.com">Example.com</a></li>
				<li>Link 2: <a href="https://example2.com">Example2.com</a></li>
				<li>Something else</li>
			</ul>
		</p>

		<table>
			<thead>
				<tr><th>Header 1</th><th>Header 2</th></tr>
			</thead>
			<tfoot>
				<tr><td>Footer 1</td><td>Footer 2</td></tr>
			</tfoot>
			<tbody>
				<tr><td>Row 1 Col 1</td><td>Row 1 Col 2</td></tr>
				<tr><td>Row 2 Col 1</td><td>Row 2 Col 2</td></tr>
			</tbody>
		</table>
	</body>
</html>`

	text, err := FromString(inputHTML, Options{PrettyTables: true})
	if err != nil {
		panic(err)
	}
	fmt.Println(text)

	// Output:
	// #+TITLE: My Mega Service
	//
	// #+NAME: Mega Service
	// [[/logo-image.jpg]]
	// [[http://jaytaylor.com/][Mega Service]]
	//
	// * Welcome to your new account on my service!
	//
	// Here is some more information:
	//
	// - Link 1: [[https://example.com][Example.com]]
	// - Link 2: [[https://example2.com][Example2.com]]
	// - Something else
	//
	// |  HEADER 1   |  HEADER 2   |
	// |-------------+-------------|
	// | Row 1 Col 1 | Row 1 Col 2 |
	// | Row 2 Col 1 | Row 2 Col 2 |
	// |-------------+-------------|
	// |  FOOTER 1   |  FOOTER 2   |
}
