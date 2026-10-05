package main

import (
	"fmt"
	"os"

	"github.com/Denzil-Briffa/kube-image-warmer/internal/warmuphelper"
)

func main() {
	if err := warmuphelper.Run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
