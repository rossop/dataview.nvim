package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	vault := flag.String("vault", "", "path to the Obsidian vault")
	query := flag.String("query", "", "dataview query, e.g. `TABLE file.name`")
	flag.Parse()

	if *vault == "" {
		fmt.Fprintln(os.Stderr, "error: --vault is required")
		flag.Usage()
		os.Exit(2)
	}

	// dv0001 scaffold: no vault reading yet. Just echo the parsed flags.
	fmt.Printf("vault=%q\nquery=%q\n", *vault, *query)
}
