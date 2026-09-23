package main

import (
	"log"
	"os"
)

func main() {
	logger := log.New(os.Stdout, "drowsyfriend: ", log.LstdFlags)
	options, err := parseRuntimeOptions(os.Args[1:])
	if err != nil {
		logger.Printf("error: %v", err)
		os.Exit(2)
	}

	if err := run(logger, options); err != nil {
		logger.Printf("error: %v", err)
		os.Exit(1)
	}
}
