package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"hermes-tools/internal/boot"
	"hermes-tools/internal/mcp"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "boot":
		cmdBoot()
	case "mcp":
		cmdMCP(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `hermes-tools — boot + MCP pre-warm/verification for the openagent umbrella

usage:
  hermes-tools boot [--manifest PATH]      run the gateway boot sequence (replaces boot.sh)
  hermes-tools mcp prewarm [--manifest PATH] [--parallel N]
                                           materialise npx/uvx package caches
  hermes-tools mcp verify [--manifest PATH] [--mode preflight|drift] [--only NAME]...
                                           handshake MCP servers and certify tools`)
}

func cmdBoot() {
	fs := flag.NewFlagSet("boot", flag.ExitOnError)
	manifest := fs.String("manifest", mcp.BootManifest, "rendered MCP manifest path")
	_ = fs.Parse(os.Args[2:])
	if err := boot.Run(*manifest); err != nil {
		log.Printf("boot: %v", err)
		os.Exit(1)
	}
}

type multiFlag []string

func (m *multiFlag) String() string { return "" }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func cmdMCP(args []string) {
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	switch args[0] {
	case "prewarm":
		fs := flag.NewFlagSet("prewarm", flag.ExitOnError)
		manifest := fs.String("manifest", mcp.BootManifest, "rendered MCP manifest path")
		parallel := fs.Int("parallel", 4, "concurrent npm materialisations")
		_ = fs.Parse(args[1:])
		os.Exit(mcp.Prewarm(*manifest, *parallel))
	case "verify":
		fs := flag.NewFlagSet("verify", flag.ExitOnError)
		manifest := fs.String("manifest", mcp.DefaultManifest, "rendered MCP manifest path")
		mode := fs.String("mode", "preflight", "preflight or drift")
		var only multiFlag
		fs.Var(&only, "only", "verify only this server (repeatable)")
		_ = fs.Parse(args[1:])
		if *mode != "preflight" && *mode != "drift" {
			log.Printf("verify: unknown mode %q (want preflight|drift)", *mode)
			os.Exit(2)
		}
		os.Exit(mcp.Execute(*manifest, *mode, only))
	default:
		usage()
		os.Exit(2)
	}
}
