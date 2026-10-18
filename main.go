// Command matching-engine is the Velexa DSE matching-engine simulator.
//
//	velexa-sim-me              start the engine
//	velexa-sim-me healthcheck  exit 0 when a local engine answers /healthz (for Docker)
//	velexa-sim-me version      print the build version
package main

import (
	"fmt"
	"os"
	_ "time/tzdata" // time zones without the OS database (minimal container images)

	cmd "velexa-sim-me/Cmd"
	"velexa-sim-me/internal/config"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(cmd.Healthcheck())
		case "version", "--version":
			fmt.Println(config.Version)
			return
		default:
			fmt.Fprintf(os.Stderr, "usage: %s [healthcheck|version]\n", os.Args[0])
			os.Exit(2)
		}
	}
	cmd.Serve()
}
