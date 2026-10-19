package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"srdashboard/rwkbericht"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 1 && (args[0] == "--check" || args[0] == "-check") {
		return runCheck()
	}
	vorlage, files, ok := parseArgs(args)
	if !ok {
		fmt.Fprintln(os.Stderr, "Verwendung: rwk.exe --check | rwk.exe [--vorlage soest|neutral] quelle.pdf [ziel.pdf]")
		return 2
	}
	in := files[0]
	out := strings.TrimSuffix(in, filepath.Ext(in)) + "_bericht.pdf"
	if len(files) == 2 {
		out = files[1]
	}
	bericht, err := readOptic(in)
	if err != nil {
		if errors.Is(err, ErrVoraussetzung) {
			fmt.Fprintln(os.Stderr, ErrVoraussetzung.Error())
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return 1
	}
	pdf, err := rwkbericht.Fill(bericht, vorlage)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	if err := os.WriteFile(out, pdf, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	fmt.Println(out)
	return 0
}

func parseArgs(args []string) (vorlage string, files []string, ok bool) {
	vorlage = rwkbericht.VorlageSoest
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--vorlage" || a == "-vorlage" {
			if i+1 >= len(args) {
				return "", nil, false
			}
			i++
			vorlage = args[i]
			continue
		}
		if strings.HasPrefix(a, "-") {
			return "", nil, false
		}
		files = append(files, a)
	}
	if len(files) < 1 || len(files) > 2 {
		return "", nil, false
	}
	return vorlage, files, true
}
