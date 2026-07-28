package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nireneko/drup/internal/app"
	drupexec "github.com/nireneko/drup/internal/exec"
)

func main() {
	// Analyses run inside containers and outlive this process when it is
	// interrupted, so take the child processes down with us.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-stop
		drupexec.KillChildren()
		fmt.Fprintf(os.Stderr, "\ndrup: interrupted (%s), stopped running commands\n", sig)
		os.Exit(130)
	}()

	if err := app.Run(os.Args[1:]); err != nil {
		drupexec.KillChildren()
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
