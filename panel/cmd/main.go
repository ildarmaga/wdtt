package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"wdtt-panel"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(panel.FormatPanelVersion())
		os.Exit(0)
	}
	if err := panel.Run(); err != nil {
		log.Fatal(err)
	}
}
