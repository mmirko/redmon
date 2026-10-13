package main

import (
	"flag"

	"github.com/mmirko/redmon/pkg/embredmon"
	"github.com/mmirko/redmon/pkg/redmon"
)

//////////////////////////////////////////////////////////////////// Application specific data

var (
	verbose = flag.Bool("v", false, "Verbose")
	debug   = flag.Bool("d", false, "Debug")

	configfile = flag.String("c", "", "Use a config file (default is to use the embedded config)")

	source = flag.String("source", "", "Choose the redmon source")
)

//////////////////////////////////////////////////////////////////// Initializzation

func init() {
	flag.Parse()
}

//////////////////////////////////////////////////////////////////// Main

func main() {
	// Load the configuration from the specified file or the embedded configuration
	c, err := redmon.LoadConfig(*configfile, embredmon.Config)
	if err != nil {
		panic(err)
	}

	// Eventually, override the verbose and debug settings if specified
	if *verbose {
		c.Verbose = true
	}
	if *debug {
		c.EmitDebug = true
	}

	// Print the loaded configuration if verbose mode is enabled
	c.Info("Loaded configuration: " + c.String())

}
