package main

import (
	"fmt"
	"log"
	"os"

	"wdtt-panel"
	"wdtt-server"
)

func main() {
	noPanel := false
	filtered := make([]string, 0, len(os.Args))
	for _, arg := range os.Args {
		switch arg {
		case "-no-panel":
			noPanel = true
		case "-version", "--version":
			fmt.Println(panel.FormatPanelVersion())
			os.Exit(0)
		default:
			filtered = append(filtered, arg)
		}
	}
	os.Args = filtered

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
