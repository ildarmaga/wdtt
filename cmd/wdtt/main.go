package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"wdtt-panel"
	"wdtt-server"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	noPanel := false
	filtered := make([]string, 0, len(os.Args))
	for _, arg := range os.Args {
		if arg == "-no-panel" {
			noPanel = true
			continue
		}
		filtered = append(filtered, arg)
	}
	os.Args = filtered
	flag.Parse()
	if *showVersion {
		fmt.Println(panel.FormatPanelVersion())
		os.Exit(0)
	}

	if !noPanel {
		go func() {
			if err := panel.Run(); err != nil {
				log.Fatalf("[PANEL] %v", err)
			}
		}()
		log.Println("[PANEL] веб-панель запущена в том же процессе")
	}

	server.Run()
}
