package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/satotake/html2org"
)

const version = "v0.0.3"

type Option struct {
	Input        string
	Output       string
	Version      bool
	BaseURL      string
	PrettyTables bool
}

func parseFlag() *Option {
	input := flag.String("i", "", "input file path (default stdin)")
	output := flag.String("o", "", "output file path (default stdout)")
	version := flag.Bool("v", false, "show version")
	baseURL := flag.String("u", "", "set BaseURL")
	table := flag.Bool("t", false, "enable PrettyTables option")
	flag.Parse()
	return &Option{
		*input, *output, *version, *baseURL, *table,
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
		fmt.Printf("html2org: HTML to org converter CLI %s", version)
		os.Exit(0)
	}

	var r io.Reader
	if opt.Input == "" {
		r = bufio.NewReader(os.Stdin)
	} else {
		r, err := os.Open(opt.Input)
		check(err)
		defer r.Close()
	}

	res, err := html2org.FromReader(r, html2org.Options{BaseURL: opt.BaseURL, PrettyTables: opt.PrettyTables})
	check(err)
	res = res + "\n"

	if opt.Output == "" {
		fmt.Println(res)
	} else {
		err := ioutil.WriteFile(opt.Output, []byte(res), 0644)
		check(err)
	}
}
