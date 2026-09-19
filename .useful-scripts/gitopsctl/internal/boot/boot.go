package boot

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"gitopsctl/internal/mcp"
)

const (
	codegraphServer  = "codegraph"
	codegraphWorkers = 4
	reposDir         = "/opt/data/repos"
	home             = "/opt/data/home"
	binDir           = "/opt/data/bin"
	miseDir          = "/opt/data/mise"
	miseConfig       = "/mise/mise.toml"
	runtimeUID       = 10000
)

func logf(format string, args ...any) {
	fmt.Printf("boot: %s\n", fmt.Sprintf(format, args...))
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

// mergedEnv applies the PVC-cache exports the script relied on (boot installs
// survive restarts; MCPs under uid 10000 reuse them instead of re-downloading).
// The exports are set on os.Environ() itself so in-process children (mcp
// prewarm, exec'd commands) see them, mirroring the script's `export`.
func mergedEnv() []string {
	set := func(k, v string) {
		os.Setenv(k, v)
	}
	set("HOME", home)
	set("NPM_CONFIG_CACHE", home+"/.npm")
	set("UV_CACHE_DIR", home+"/.cache/uv")
	set("UV_TOOL_DIR", home+"/.local/share/uv/tools")
	set("UV_TOOL_BIN_DIR", home+"/.local/bin")
	set("MISE_DATA_DIR", miseDir)
	set("MISE_CACHE_DIR", "/opt/data/mise-cache")
	set("MISE_CONFIG_FILE", miseConfig)
	os.Setenv("PATH", "/opt/data/mise/shims:"+binDir+":/opt/data/home/.local/bin:"+os.Getenv("PATH"))
	return os.Environ()
}

func runEnv(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func executable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode()&0o111 != 0
}

func quietOK(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}

func installMise(env []string) {
	if executable(filepath.Join(binDir, "mise")) {
		return
	}
	// official bootstrap; MISE_INSTALL_PATH pins it to the PVC so it survives
	// restarts like the mise data dirs.
	cmd := exec.Command("sh", "-c",
		"curl -fsSL https://mise.run | MISE_INSTALL_PATH="+binDir+"/mise sh")
	cmd.Env = env
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		logf("mise install failed: %v", err)
	}
}

// Desktop-E2E X11 toolchain (mirrors openkite e2e.yml) plus the Rust
// link-stage dev libraries (.pc files) local `cargo test` needs.
const systemDepsScript = `set -e
apt-get update -qq
apt-get install -y --no-install-recommends xvfb xdotool openbox imagemagick dbus-x11 bats
apt-get install -y --no-install-recommends libwebkit2gtk-4.1-dev libgtk-3-dev \
  libglib2.0-dev libayatana-appindicator3-dev librsvg2-dev libxdo-dev libssl-dev
`

// systemDeps installs systemDepsScript detached: the run takes ~6 minutes, and
// blocking handover on it puts the rollout past the Deployment's 600s progress
// deadline, which ArgoCD reports as a failed sync.
func systemDeps(env []string) {
	if quietOK("bats", "--version") && quietOK("xdotool", "--version") && quietOK("pkg-config", "--exists", "glib-2.0") {
		return
	}
	cmd := exec.Command("sh", "-c", systemDepsScript)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logf("system packages: %v", err)
		return
	}
	logf("system packages installing in the background (pid %d)", cmd.Process.Pid)
}

func linkShims(env []string) {
	// /init resets PATH — symlinks into /opt/data/bin (first on PATH) are what
	// reach MCPs; direct downloads below cover tools mise doesn't ship.
	entries, err := os.ReadDir(miseDir + "/shims")
	if err != nil {
		return
	}
	for _, e := range entries {
		src := filepath.Join(miseDir, "shims", e.Name())
		if fi, err := os.Stat(src); err != nil || fi.Mode()&0o111 == 0 {
			continue
		}
		_ = os.Remove(filepath.Join(binDir, e.Name()))
		_ = os.Symlink(src, filepath.Join(binDir, e.Name()))
	}
}

func extractTarball(url, dir string, names ...string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if len(names) > 0 && !contains(names, hdr.Name) {
			continue
		}
		path := filepath.Join(dir, filepath.Base(hdr.Name))
		out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		out.Close()
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// directDownloads covers tools absent from the mise registry, keeping the
// exact pinned releases (deno v2.9.5 matches drawio's deno.lock v5).
func directDownloads(env []string) {
	if !executable(binDir + "/obscura") {
		_ = extractTarball(
			"https://github.com/h4ckf0r0day/obscura/releases/download/v0.2.0/obscura-aarch64-linux-stealth.tar.gz",
			binDir)
	}
	if !quietOK("agent-reach", "--help") {
		_ = runEnv(env, "uv", "tool", "install",
			"https://github.com/Panniantong/agent-reach/archive/main.zip")
	}
	os.MkdirAll(home+"/.config/yt-dlp", 0o755)
	cfg := home + "/.config/yt-dlp/config"
	if data, _ := os.ReadFile(cfg); !strings.Contains(string(data), "--js-runtimes node") {
		if f, err := os.OpenFile(cfg, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			_, _ = f.WriteString("--js-runtimes node\n")
			f.Close()
		}
	}
	// OCR-first vision — mise task self-bootstrapping the py3.12 venv on first
	// use; rapidocr-onnxruntime because native paddlepaddle crashes on aarch64.
	_ = os.WriteFile(binDir+"/paddle-ocr",
		[]byte("#!/bin/bash\nexec mise run paddle-ocr -- \"$@\"\n"), 0o755)

	if !executable(binDir+"/deno") && !executable(home+"/.deno/bin/deno") {
		// install.sh needs unzip/7z (absent) — release zip extracted via Go's
		// archive/zip instead.
		zipPath := "/tmp/deno.zip"
		resp, err := http.Get("https://github.com/denoland/deno/releases/download/v2.9.5/deno-arm64-unknown-linux-gnu.zip")
		if err == nil {
			out, _ := os.Create(zipPath)
			_, _ = io.Copy(out, resp.Body)
			resp.Body.Close()
			out.Close()
			if zr, err := zip.OpenReader(zipPath); err == nil {
				for _, f := range zr.File {
					if f.Name == "deno" {
						rc, _ := f.Open()
						dst, _ := os.OpenFile(home+"/.deno/bin/deno",
							os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
						_, _ = io.Copy(dst, rc)
						rc.Close()
						dst.Close()
					}
				}
				zr.Close()
			}
		}
		_ = os.Remove(zipPath)
	}
	if executable(home + "/.deno/bin/deno") {
		_ = os.Remove(binDir + "/deno")
		_ = os.Symlink(home+"/.deno/bin/deno", binDir+"/deno")
	}
	// drawio MCP needs a local checkout (remote-URL run never discovers
	// deno.json).
	if !exists("/opt/data/drawio-mcp-server/.git") {
		_ = runEnv(env, "git", "clone", "--depth", "1",
			"https://github.com/simonkurtz-MSFT/drawio-mcp-server",
			"/opt/data/drawio-mcp-server")
	}
	// Doppler CLI; gpgv verify skipped (no gnupg as uid 10000).
	if !executable(binDir + "/doppler") {
		if err := extractTarball(
			"https://cli.doppler.com/download?os=linux&arch=arm64&format=tar",
			binDir, "doppler"); err != nil {
			logf("doppler download failed: %v", err)
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func prewarm(manifest string) {
	if !exists(manifest) {
		logf("mcp manifest %s missing — caches not pre-warmed", manifest)
		return
	}
	// Non-fatal: a pre-warm failure must never block the gateway.
	if rc := mcp.Prewarm(manifest, 4); rc != 0 {
		logf("MCP cache pre-warm reported errors (non-fatal)")
	}
}

// runTimeout is runEnv with a deadline, so a registry hang cannot block handover.
func runTimeout(env []string, seconds int, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CodegraphFrom derives the npx spec and cache dir from the rendered manifest,
// so the pin lives in values.yaml only.
func CodegraphFrom(servers map[string]*mcp.Server) (string, string) {
	s := servers[codegraphServer]
	if s == nil || !s.IsEnabled() || !s.Stdio() {
		return "", ""
	}
	_, spec := mcp.PackageSpec(*s)
	return spec, mcp.NPMCacheFor(*s)
}

type codegraphCommand struct {
	args   []string
	verb   string
	budget int
}

// codegraphCommandFor returns the npx invocation, verb and timeout for indexing path.
func codegraphCommandFor(spec, path string) codegraphCommand {
	if exists(filepath.Join(path, ".codegraph")) {
		return codegraphCommand{args: []string{"-y", spec, "sync", path}, verb: "sync", budget: 120}
	}
	return codegraphCommand{args: []string{"-y", spec, "init", "-y", path}, verb: "init", budget: 300}
}

// IndexRepository runs codegraph init-or-sync for path and returns the verb used.
func IndexRepository(base []string, spec, cache, path string) (string, error) {
	cmd := codegraphCommandFor(spec, path)
	env := append([]string(nil), base...)
	env = setEnv(env, "CODEGRAPH_TELEMETRY", "0")
	env = setEnv(env, "npm_config_cache", cache)
	env = setEnv(env, "npm_config_loglevel", "error")
	if err := runTimeout(env, cmd.budget, "npx", cmd.args...); err != nil {
		return cmd.verb, err
	}
	return cmd.verb, nil
}

// codegraphIndexes indexes each git repo under reposDir, or catches an existing
// index up.
func codegraphIndexes(env []string, manifest string) {
	servers, err := mcp.LoadManifest(manifest)
	if err != nil {
		logf("codegraph: %v", err)
		return
	}
	spec, cache := CodegraphFrom(servers)
	if spec == "" {
		return
	}
	if !quietOK("npx", "--version") {
		logf("npx missing — codegraph indexes skipped")
		return
	}
	entries, err := os.ReadDir(reposDir)
	if err != nil {
		return
	}
	// One npx per repo, each in its own process tree, so the repos index
	// concurrently instead of queueing behind the slowest one.
	var wg sync.WaitGroup
	workers := make(chan struct{}, codegraphWorkers)
	for _, e := range entries {
		repo := filepath.Join(reposDir, e.Name())
		if !e.IsDir() || !exists(filepath.Join(repo, ".git")) {
			continue
		}
		wg.Add(1)
		go func(name, repo string) {
			defer wg.Done()
			workers <- struct{}{}
			defer func() { <-workers }()
			verb, err := IndexRepository(env, spec, cache, repo)
			if err != nil {
				logf("codegraph %s %s failed (non-fatal): %v", verb, name, err)
				return
			}
			logf("codegraph %s: %s", verb, name)
		}(e.Name(), repo)
	}
	wg.Wait()
}

func chownTree() {
	// uid 10000 owns the runtime tree (npx/uvx/mise shims EACCES otherwise);
	// missing dirs are fine on first boots. The trees are disjoint, so the
	// walks run concurrently: on the PVC they are the slowest inline stage.
	var wg sync.WaitGroup
	walk := func(path string, onlyRootOwned bool) {
		defer wg.Done()
		_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !onlyRootOwned {
				_ = os.Chown(p, runtimeUID, runtimeUID)
				return nil
			}
			if fi, err := d.Info(); err == nil {
				if sys, ok := fi.Sys().(*syscall.Stat_t); ok && sys.Uid == 0 {
					_ = os.Chown(p, runtimeUID, runtimeUID)
				}
			}
			return nil
		})
	}
	for _, path := range []string{
		home + "/.npm", home + "/.cache", home + "/.deno", home + "/.local",
		home + "/.config", miseDir, "/opt/data/mise-cache", "/opt/data/npm-global",
		"/opt/data/.npm-mcp",
	} {
		wg.Add(1)
		go walk(path, false)
	}
	// Anything still root-owned in agent-writable trees (root-owned git object
	// dirs broke commits in a shared worktree). Targeted, not recursive over
	// the whole volume, so boot stays fast on multi-GB data.
	for _, path := range []string{
		"/opt/data/repos", "/opt/data/wt", "/opt/data/.local",
		"/opt/data/.config", "/opt/data/.omo",
	} {
		wg.Add(1)
		go walk(path, true)
	}
	wg.Wait()
}

func opencodeSetup(env []string) {
	// opencode CLI → PVC (npm; survives restarts like the mise shims).
	if !executable(binDir + "/opencode") {
		env = setEnv(env, "NPM_CONFIG_PREFIX", "/opt/data/npm-global")
		_ = runEnv(env, "npm", "install", "-g", "opencode-ai")
		_ = os.Symlink("/opt/data/npm-global/bin/opencode", binDir+"/opencode")
	}
	// Plugin: clone into the default profile's plugins dir (HERMES_HOME=/opt/data).
	if !exists("/opt/data/plugins/opencode/.git") {
		_ = os.MkdirAll("/opt/data/plugins", 0o755)
		_ = runEnv(env, "git", "clone", "--depth", "1",
			"https://github.com/zaycruz/hermes-opencode-plugin.git",
			"/opt/data/plugins/opencode")
	}
	// Plugin skill → skills tree (opencode-driven-development).
	_ = os.MkdirAll("/opt/data/skills/software-development/opencode-driven-development", 0o755)
	if src, err := os.ReadFile("/opt/data/plugins/opencode/SKILL.md"); err == nil {
		_ = os.WriteFile("/opt/data/skills/software-development/opencode-driven-development/SKILL.md",
			src, 0o644)
	}
	// OMO normalizes ~/.omo/omo.jsonc (migrations, model dedupe) → copy the
	// mounted template to a writable path each boot, then chown to uid 10000.
	_ = os.MkdirAll("/opt/data/.omo", 0o755)
	if tmpl, err := os.ReadFile("/opt/opencode/omo.jsonc"); err == nil {
		_ = os.WriteFile("/opt/data/.omo/omo.jsonc", tmpl, 0o644)
	}
	for _, path := range []string{"/opt/data/plugins", "/opt/data/.omo"} {
		_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err == nil {
				_ = os.Chown(p, runtimeUID, runtimeUID)
			}
			return nil
		})
	}
}

// Run executes the boot sequence, then replaces the process with hermes
// (/init hermes gateway run) so PID-1 semantics (signal handling, process
// supervision) stay hers. Returns only on a hard failure before handover.
func Run(manifest string) error {
	env := mergedEnv()
	installMise(env)
	_ = runEnv(env, "mise", "install", "-y")
	if !quietOK("mise", "ls") {
		logf("mise config failed to parse — see " + miseConfig)
	}
	systemDeps(env)
	linkShims(env)
	directDownloads(env)
	prewarm(manifest)
	codegraphIndexes(env, manifest)
	chownTree()
	opencodeSetup(env)
	logf("handing over to hermes")
	return syscall.Exec("/init", []string{"/init", "hermes", "gateway", "run"}, env)
}

// PrewarmManifest returns the manifest path used when boot pre-warms MCP
// caches: the mcp-manifest ConfigMap is mounted at /opt/data/mcp-verify in
// the gateway pod.
func PrewarmManifest() string { return "/opt/data/mcp-verify/mcp-manifest" }
