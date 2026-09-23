package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

type runtimeOptions struct {
	MockData bool
}

func parseRuntimeOptions(args []string) (runtimeOptions, error) {
	var options runtimeOptions

	flags := flag.NewFlagSet("drowsyfriend", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(
		&options.MockData,
		"mock-data",
		false,
		"use deterministic local mock policy data",
	)
	if err := flags.Parse(args); err != nil {
		return runtimeOptions{}, err
	}
	if flags.NArg() != 0 {
		return runtimeOptions{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}

	return options, nil
}
