package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/TykTechnologies/graphql-go-tools/pkg/astprinter"
	"github.com/TykTechnologies/graphql-go-tools/pkg/openapi"
)

type arguments struct {
	output   string
	document string
	help     bool
}

func usage() {
	var msg = `Usage: oas-to-graphql [options] ...

Produce a GraphQL schema based on an OpenAPI document

Options:
  -h, --help          Print this message and exit.
  -d  --document      OAS file path or remote url.
  -o, --output        Save schema to path.
`
	_, err := fmt.Fprintf(os.Stdout, msg)
	if err != nil {
		panic(err)
	}
}

func main() {
	args := &arguments{}
	log.SetFlags(0)

	// Parse command line parameters
	f := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	f.SetOutput(ioutil.Discard)
	f.BoolVar(&args.help, "h", false, "")
	f.BoolVar(&args.help, "help", false, "")
	f.StringVar(&args.document, "d", "", "")
	f.StringVar(&args.document, "document", "", "")
	f.StringVar(&args.output, "o", "", "")
	f.StringVar(&args.output, "output", "", "")

	if err := f.Parse(os.Args[1:]); err != nil {
		log.Fatalf("Failed to parse flags: %v", err)
	}

	if args.help {
		usage()
		return
	}

	if args.document == "" {
		log.Fatal("OpenAPI document not provided")
	}

	if args.output == "" {
		log.Fatal("Output path not provided")
	}

	var document []byte
	var err error
	if strings.HasPrefix(args.document, "http") {
		response, err := http.Get(args.document)
		if err != nil {
			log.Fatalf("Failed to fetch OpenAPI document from %s: %s", args.document, err)
		}
		document, err = ioutil.ReadAll(response.Body)
		if err != nil {
			log.Fatalf("Failed to read OpenAPI document from response body: %s", err)
		}
	} else {
		document, err = os.ReadFile(args.document)
		if err != nil {
			log.Fatalf("Failed to read OpenAPI document: %s: %s", args.document, err)
		}
	}

	graphqlDocument, report := openapi.ImportOpenAPIDocumentByte(document)
	if report.HasErrors() {
		log.Fatalf("Failed to import OpenAPI document: %s", report.Error())
	}

	w := &bytes.Buffer{}
	err = astprinter.PrintIndent(graphqlDocument, nil, []byte("  "), w)
	output, err := os.Create(args.output)
	if err != nil {
		log.Fatalf("Failed to create %s: %s", args.output, err)
	}
	_, err = output.Write(w.Bytes())
	if err != nil {
		log.Fatalf("Failed to write GraphQL document %s: %s", args.output, err)
	}

	log.Printf("oas-to-graphql successfully saved your schema at %s", args.output)
}
