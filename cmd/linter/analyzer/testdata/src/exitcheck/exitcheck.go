package main

import (
	mylog "log"
	myos "os"
)

// helper calls os.Exit and log.Fatal via aliases — must still be reported.
func helper() {
	myos.Exit(1)        // want `call to myos.Exit is forbidden outside of main.main`
	mylog.Fatal("oops") // want `call to mylog.Fatal is forbidden outside of main.main`
	mylog.Fatalf("oops %d", 1) // want `call to mylog.Fatalf is forbidden outside of main.main`
	mylog.Fatalln("oops")     // want `call to mylog.Fatalln is forbidden outside of main.main`
}

func main() {
	// These are inside main() — must NOT be reported.
	if err := run(); err != nil {
		mylog.Fatalf("error: %v", err)
	}
	myos.Exit(0)
}

func run() error { return nil }
