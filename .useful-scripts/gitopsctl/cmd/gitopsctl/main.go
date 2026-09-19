package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gitopsctl/internal/boot"
	"gitopsctl/internal/mcp"
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
	case "install":
		cmdInstall(os.Args[2:])
	case "codegraph":
		cmdCodegraph(os.Args[2:])
	case "mcp":
		cmdMCP(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `gitopsctl — boot + MCP pre-warm/verification for the openagent umbrella

usage:
  gitopsctl boot [--manifest PATH]      run the gateway boot sequence (replaces boot.sh)
  gitopsctl install DEST                 copy this executable to DEST (initContainer seeding)
  gitopsctl codegraph index PATH [--manifest PATH]
                                           index one repository (init or sync)
  gitopsctl mcp prewarm [--manifest PATH] [--parallel N]
                                           materialise npx/uvx package caches
  gitopsctl mcp verify [--manifest PATH] [--mode preflight|drift|validate] [--only NAME]...
                                           handshake MCP servers and certify tools
                                           (validate = static manifest policy lint, no network)`)
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

// cmdInstall copies the running executable to a single destination path. It
// lets a scratch-based tools image seed a binary into an emptyDir from an
// initContainer, where no shell or cp exists.
func cmdInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
		os.Exit(2)
	}
	if err := install(fs.Arg(0)); err != nil {
		log.Printf("install: %v", err)
		os.Exit(1)
	}
}

func install(dest string) error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if dir := filepath.Dir(dest); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	mode := os.FileMode(0o755)
	if info, err := in.Stat(); err == nil {
		mode = info.Mode().Perm()
	}
	return os.Chmod(dest, mode|0o111)
}

func cmdCodegraph(args []string) {
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	switch args[0] {
	case "index":
		fs := flag.NewFlagSet("index", flag.ExitOnError)
		manifest := fs.String("manifest", mcp.BootManifest, "rendered MCP manifest path")
		_ = fs.Parse(manifestFlagFirst(args[1:]))
		if fs.NArg() != 1 {
			usage()
			os.Exit(2)
		}
		cmdCodegraphIndex(*manifest, fs.Arg(0))
	default:
		usage()
		os.Exit(2)
	}
}

// manifestFlagFirst moves --manifest ahead of PATH for the flag package.
func manifestFlagFirst(args []string) []string {
	flags := []string{}
	rest := []string{}
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--manifest" || arg == "-manifest":
			if i+1 < len(args) {
				flags = append(flags, arg, args[i+1])
				i++
			} else {
				flags = append(flags, arg)
			}
		case strings.HasPrefix(arg, "--manifest=") || strings.HasPrefix(arg, "-manifest="):
			flags = append(flags, arg)
		default:
			rest = append(rest, arg)
		}
	}
	return append(flags, rest...)
}

func cmdCodegraphIndex(manifest, path string) {
	servers, err := mcp.LoadManifest(manifest)
	if err != nil {
		log.Printf("codegraph: %v", err)
		os.Exit(1)
	}
	spec, cache := boot.CodegraphFrom(servers)
	if spec == "" {
		log.Printf("codegraph: no enabled codegraph server in %s", manifest)
		os.Exit(1)
	}
	verb, err := boot.IndexRepository(os.Environ(), spec, cache, path)
	if err != nil {
		log.Printf("codegraph %s: %v", verb, err)
		os.Exit(1)
	}
	if verb == "sync" {
		fmt.Printf("codegraph: already up to date: %s\n", path)
		return
	}
	fmt.Printf("codegraph: indexed: %s\n", path)
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
		mode := fs.String("mode", "preflight", "preflight, drift or validate")
		var only multiFlag
		fs.Var(&only, "only", "verify only this server (repeatable)")
		_ = fs.Parse(args[1:])
		if *mode != "preflight" && *mode != "drift" && *mode != "validate" {
			log.Printf("verify: unknown mode %q (want preflight|drift|validate)", *mode)
			os.Exit(2)
		}
		if *mode == "validate" {
			os.Exit(mcp.ExecuteValidate(*manifest))
		}
		os.Exit(mcp.Execute(*manifest, *mode, only))
	default:
		usage()
		os.Exit(2)
	}
}
