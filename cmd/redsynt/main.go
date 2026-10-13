package main

import (
	"flag"
)

//////////////////////////////////////////////////////////////////// Application specific data

var (
	verbose = flag.Bool("v", false, "Verbose")
	debug   = flag.Bool("d", false, "Debug")

	configfile = flag.String("c", "", "Use a config file (default is to use the embedded config)")

	source = flag.String("source", "", "Choose the redmon source")

	fullkey   = flag.String("k", "", "Key (alternate + metric) to consider")
	metric    = flag.String("m", "", "Metric to consider")
	alternate = flag.String("a", "", "Specify an alterate metric prefix other than the hostname")

	save   = flag.Bool("s", false, "Save configurations as json")
	load   = flag.Bool("l", false, "Load configurations as josn")
	remove = flag.Bool("r", false, "Remove a given metric")

	ttl = flag.Int("t", 0, "Set the TTL for the metric config")

	list = flag.Bool("f", false, "Full list all server metrics")
)

//////////////////////////////////////////////////////////////////// Initializzation

func init() {
	flag.Parse()
}

//////////////////////////////////////////////////////////////////// Main

func main() {
}
