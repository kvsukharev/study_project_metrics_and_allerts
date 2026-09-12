package main

import (
	"log"
	"os"
)

// helper calls os.Exit and log.Fatal outside of main — must be reported.
func helper() {
	os.Exit(1)        // want `call to os.Exit is forbidden outside of main.main`
	log.Fatal("oops") // want `call to log.Fatal is forbidden outside of main.main`
	log.Fatalf("oops %d", 1) // want `call to log.Fatalf is forbidden outside of main.main`
	log.Fatalln("oops")     // want `call to log.Fatalln is forbidden outside of main.main`
}

func main() {
	// These are inside main() — must NOT be reported.
	if err := run(); err != nil {
		log.Fatalf("error: %v", err)
	}
	os.Exit(0)
}

func run() error { return nil }
