package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/satotake/html2org"
)

//go:embed VERSION
var version string

type Option struct {
	Input           string
	Output          string
	Version         bool
	BaseURL         string
	PrettyTables    bool
	Noscript        bool
	Check           bool
	InternalLinks   bool
	ShowLongDataURL bool
}

func parseFlag() *Option {
	input := flag.String("i", "", "input file path (default stdin)")
	output := flag.String("o", "", "output file path (default stdout)")
	version := flag.Bool("v", false, "show version")
	baseURL := flag.String("u", "", "set BaseURL")
	table := flag.Bool("t", false, "enable PrettyTables option")
	noscript := flag.Bool("noscript", false, "show content inside noscript tag")
	check := flag.Bool("c", false, "sniff content and throw error if it is guessed as non-html")
	internalLinks := flag.Bool("l", false, "show internal link destinations if the link exists.")
	showLongDataURL := flag.Bool("image-data-url", false, "show all data url in img tags")
	flag.Parse()

	return &Option{
		*input,
		*output,
		*version,
		*baseURL,
		*table,
		*noscript,
		*check,
		*internalLinks,
		*showLongDataURL,
	}
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	opt := parseFlag()

	if opt.Version {
		fmt.Printf("html2org: HTML to org converter CLI v%s", version)
		os.Exit(0)
	}

	var err error
	var r io.Reader
	if opt.Input == "" {
		r = (os.Stdin)
	} else {
		f, err := os.Open(opt.Input)
		check(err)
		defer f.Close()
		r = (f)
	}

	if opt.Check {
		b := make([]byte, 512)
		_, err = r.Read(b)
		check(err)
		err = checkNonHtmlContent(b)
		check(err)
		reused := bytes.NewReader(b)
		r = io.MultiReader(reused, r)
	}

	res, err := html2org.FromReader(r, html2org.Options{
		BaseURL:         opt.BaseURL,
		PrettyTables:    opt.PrettyTables,
		ShowNoscripts:   opt.Noscript,
		InternalLinks:   opt.InternalLinks,
		ShowLongDataURL: opt.ShowLongDataURL,
	})
	check(err)
	res = res + "\n"

	if opt.Output == "" {
		fmt.Println(res)
	} else {
		err := ioutil.WriteFile(opt.Output, []byte(res), 0644)
		check(err)
	}
}

func checkNonHtmlContent(b []byte) error {
	ct := http.DetectContentType(b)
	if !(strings.Contains(ct, "text/html") || strings.Contains(ct, "text/xml")) {
		return fmt.Errorf("non-html file: %s", ct)
	}
	return nil
}
