package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func main() {
	schemaPath := flag.String("schema", "", "Review JSON Schema path")
	documentPath := flag.String("document", "", "Review document path")
	flag.Parse()
	if *schemaPath == "" || *documentPath == "" {
		fmt.Fprintln(os.Stderr, "--schema and --document are required")
		os.Exit(2)
	}

	schemaBytes, err := os.ReadFile(*schemaPath)
	if err != nil {
		fail(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	if err := compiler.AddResource("review.schema.json", bytes.NewReader(schemaBytes)); err != nil {
		fail(err)
	}
	schema, err := compiler.Compile("review.schema.json")
	if err != nil {
		fail(err)
	}

	documentBytes, err := os.ReadFile(*documentPath)
	if err != nil {
		fail(err)
	}
	var document any
	if err := json.Unmarshal(documentBytes, &document); err != nil {
		fail(err)
	}
	if err := schema.Validate(document); err != nil {
		fail(err)
	}
	fmt.Println("schema_valid=true")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
