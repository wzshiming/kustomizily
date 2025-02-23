package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/wzshiming/kustomizily"
)

var (
	inputFile string
	outputDir string
	dryRun    bool
)

func init() {
	flag.StringVar(&inputFile, "i", "-", "Input k8s YAML file")
	flag.StringVar(&outputDir, "o", "./kustomizily", "Output directory")
	flag.BoolVar(&dryRun, "d", false, "Dry run mode")
	flag.Parse()
}

func main() {
	if flag.NArg() > 0 {
		fmt.Println("Unrecognized arguments:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if inputFile == "" {
		fmt.Println("Input file is required")
		flag.PrintDefaults()
		os.Exit(1)
	}

	var reader io.Reader
	if inputFile == "-" {
		reader = os.Stdin
	} else {
		f, err := os.Open(inputFile)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer f.Close()
		reader = f
	}

	var writeFile func(dir string, name string, data []byte) error
	if dryRun {
		writeFile = kustomizily.NewDryRunFS(outputDir).WriteFile
	} else {
		writeFile = kustomizily.NewFS(outputDir).WriteFile
	}

	h := kustomizily.NewBuilder()

	err := h.Process(reader)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = h.Build(writeFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
