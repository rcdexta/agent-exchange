package main

import (
	"fmt"
	"github.com/summationai/agent-exchange/internal/ax"
	"os"
)

func main() {
	if e := ax.Main(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, "ax:", e)
		os.Exit(1)
	}
}
