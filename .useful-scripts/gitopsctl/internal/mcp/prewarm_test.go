package mcp

import "testing"

var drawioCommand = Server{
	Command: "sh",
	Args: []string{
		"-c",
		"cd /opt/data/drawio-mcp-server && exec /opt/data/bin/deno run --config /opt/data/drawio-mcp-server/deno.json -P --allow-read --allow-env --allow-net src/index.ts --transport stdio",
	},
}

func TestDenoSpec(t *testing.T) {
	tests := []struct {
		name     string
		server   Server
		wantOK   bool
		wantBin  string
		wantDir  string
		wantCfg  string
		wantEntr string
	}{
		{
			name:     "shell-wrapped local checkout",
			server:   drawioCommand,
			wantOK:   true,
			wantBin:  "/opt/data/bin/deno",
			wantDir:  "/opt/data/drawio-mcp-server",
			wantCfg:  "/opt/data/drawio-mcp-server/deno.json",
			wantEntr: "src/index.ts",
		},
		{
			name:     "bare deno run, flag before entry",
			server:   Server{Command: "deno", Args: []string{"run", "--allow-net", "main.ts"}},
			wantOK:   true,
			wantBin:  "deno",
			wantEntr: "main.ts",
		},
		{
			name:     "transport value is not the entry",
			server:   Server{Command: "deno", Args: []string{"run", "--transport", "stdio", "server.ts"}},
			wantOK:   true,
			wantBin:  "deno",
			wantEntr: "server.ts",
		},
		{
			name:   "npx server",
			server: Server{Command: "npx", Args: []string{"-y", "@colbymchenry/codegraph@1.6.0", "serve", "--mcp"}},
			wantOK: false,
		},
		{
			name:   "deno without an entry",
			server: Server{Command: "deno", Args: []string{"run", "--allow-net"}},
			wantOK: false,
		},
		{
			name:   "uvx server",
			server: Server{Command: "uvx", Args: []string{"plane-mcp-server==0.3.2", "stdio"}},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, ok := DenoSpec(tt.server)
			if ok != tt.wantOK {
				t.Fatalf("DenoSpec ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if inv.Bin != tt.wantBin {
				t.Errorf("Bin = %q, want %q", inv.Bin, tt.wantBin)
			}
			if inv.Dir != tt.wantDir {
				t.Errorf("Dir = %q, want %q", inv.Dir, tt.wantDir)
			}
			if inv.Config != tt.wantCfg {
				t.Errorf("Config = %q, want %q", inv.Config, tt.wantCfg)
			}
			if inv.Entry != tt.wantEntr {
				t.Errorf("Entry = %q, want %q", inv.Entry, tt.wantEntr)
			}
		})
	}
}

func TestPackageSpecIgnoresDeno(t *testing.T) {
	for name, server := range map[string]Server{
		"drawio": drawioCommand,
		"bare":   {Command: "deno", Args: []string{"run", "main.ts"}},
	} {
		kind, spec := PackageSpec(server)
		if kind != "" || spec != "" {
			t.Errorf("%s: PackageSpec = (%q, %q), want (\"\", \"\")", name, kind, spec)
		}
	}
}
