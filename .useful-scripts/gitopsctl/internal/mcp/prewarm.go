package mcp

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	prewarmMarker    = ".mcp-prewarm"
	prewarmNPMTimout = 300 * time.Second
	prewarmUVTimeout = 600 * time.Second
)

func prewarmLog(format string, args ...any) {
	fmt.Printf("[mcp-prewarm] %s\n", fmt.Sprintf(format, args...))
}

// PackageSpec returns (kind, spec) for an npx/uvx server, else ("", ""),
// handling shell-wrapped forms (sh -c "exec npx -y pkg | grep jsonrpc").
func PackageSpec(server Server) (string, string) {
	parts := []string{server.Command}
	parts = append(parts, server.Args...)
	tokens := tokenRe.Split(strings.Join(parts, " "), -1)
	for i, token := range tokens {
		if token != "npx" && token != "uvx" {
			continue
		}
		kind := "npm"
		if token == "uvx" {
			kind = "uv"
		}
		for _, candidate := range tokens[i+1:] {
			if candidate == "" || strings.HasPrefix(candidate, "-") || candidate == "exec" || candidate == "run" || candidate == "dlx" {
				continue
			}
			return kind, candidate
		}
	}
	return "", ""
}

var tokenRe = regexp.MustCompile(`[\s;|&()]+`)

// runBounded runs cmd in its own session/process group and SIGKILLs the group
// on timeout, so nothing can outlive the bound.
func runBounded(cmd *exec.Cmd, timeout time.Duration) (int, string) {
	cmd.Stdin = nil
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return -1, err.Error()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		return cmd.ProcessState.ExitCode(), errBuf.String()
	case <-time.After(timeout):
		pgid := cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		time.Sleep(2 * time.Second)
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-done
		return -1, fmt.Sprintf("timed out after %s (process group killed)", timeout)
	}
}

func materialiseNPM(name, spec, cache string, env []string) bool {
	marker := filepath.Join(cache, prewarmMarker)
	if _, err := os.Stat(marker); err == nil {
		if data, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(data)) == spec {
			if info, err := os.Stat(filepath.Join(cache, "_npx")); err == nil && info.IsDir() {
				prewarmLog("%s: cache already warm (%s)", name, spec)
				return true
			}
		}
	}
	_ = os.RemoveAll(filepath.Join(cache, "_npx"))
	if err := os.MkdirAll(cache, 0o755); err != nil {
		prewarmLog("%s: cannot create cache %s: %v", name, cache, err)
		return false
	}
	cmd := exec.Command("npm", "exec", "--yes", "--package="+spec, "--", "true")
	cmd.Env = append(env, "NPM_CONFIG_CACHE="+cache)
	rc, errOut := runBounded(cmd, prewarmNPMTimout)
	if rc == 0 {
		if err := os.WriteFile(marker, []byte(spec), 0o644); err != nil {
			prewarmLog("%s: cannot write marker: %v", name, err)
			return false
		}
		prewarmLog("%s: materialised %s into %s", name, spec, cache)
		return true
	}
	prewarmLog("%s: FAILED to materialise %s (rc=%d) %s", name, spec, rc, snippet(errOut))
	return false
}

func materialiseUV(name, spec string, env []string) bool {
	cmd := exec.Command("uv", "tool", "install", "--quiet", spec)
	cmd.Env = env
	rc, errOut := runBounded(cmd, prewarmUVTimeout)
	if rc == 0 {
		prewarmLog("%s: materialised %s (uv tool)", name, spec)
		return true
	}
	prewarmLog("%s: FAILED to materialise %s (rc=%d) %s", name, spec, rc, snippet(errOut))
	return false
}

func cmdline(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(string(data), "\x00", " "))
}

func ppid(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	after := data
	if idx := bytes.LastIndexByte(after, ')'); idx >= 0 {
		fields := strings.Fields(string(after[idx+1:]))
		if len(fields) > 1 {
			if n, err := strconv.Atoi(fields[1]); err == nil {
				return n
			}
		}
	}
	return 0
}

func looksLikePrewarmOrphan(cmd string) bool {
	if !strings.Contains(cmd, "_npx") && !strings.Contains(cmd, ".npm-mcp") {
		return false
	}
	argv0 := filepath.Base(strings.SplitN(cmd, " ", 2)[0])
	return argv0 == "node" || argv0 == "npm" || argv0 == "npx" || strings.Contains(cmd, "node_modules")
}

// sweepOrphans kills reparented (ppid==1) leftovers from older pre-warm boots.
func sweepOrphans() [][2]string {
	var killed [][2]string
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return killed
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		cmd := cmdline(pid)
		if cmd == "" || ppid(pid) != 1 {
			continue
		}
		if looksLikePrewarmOrphan(cmd) {
			if err := syscall.Kill(pid, syscall.SIGKILL); err == nil {
				if len(cmd) > 160 {
					cmd = cmd[:160]
				}
				killed = append(killed, [2]string{strconv.Itoa(pid), cmd})
			}
		}
	}
	return killed
}

// Prewarm materialises the npx/uvx package caches at boot without starting any
// server. Returns 1 when packages failed to materialise (non-fatal at boot).
func Prewarm(manifestPath string, parallel int) int {
	servers, err := LoadManifest(manifestPath)
	if err != nil {
		prewarmLog("manifest %s unavailable (%v); skipping pre-warm", manifestPath, err)
		return 0
	}

	type npmJob struct{ name, spec, cache string }
	var npmJobs []npmJob
	var uvJobs [][2]string
	for name, server := range servers {
		if server == nil {
			continue
		}
		kind, spec := PackageSpec(*server)
		if kind == "" {
			continue
		}
		if kind == "npm" {
			npmJobs = append(npmJobs, npmJob{name, spec, NPMCacheFor(*server)})
		} else {
			uvJobs = append(uvJobs, [2]string{name, spec})
		}
	}

	// Sweep BEFORE materialising so we never touch our own children.
	killed := sweepOrphans()
	for _, k := range killed {
		prewarmLog("killed orphan pid %s: %s", k[0], k[1])
	}
	prewarmLog("swept %d orphaned pre-warm process(es)", len(killed))

	env := os.Environ()
	failures := 0
	done := make(chan bool, len(npmJobs))
	sem := make(chan struct{}, parallel)
	for _, job := range npmJobs {
		go func(job npmJob) {
			sem <- struct{}{}
			defer func() { <-sem }()
			done <- materialiseNPM(job.name, job.spec, job.cache, env)
		}(job)
	}
	for range npmJobs {
		if !<-done {
			failures++
		}
	}
	for _, job := range uvJobs {
		if !materialiseUV(job[0], job[1], env) {
			failures++
		}
	}

	remaining := sweepOrphans()
	prewarmLog("%d pre-warm orphan(s) remain after materialisation", len(remaining))
	if failures > 0 {
		prewarmLog("%d package(s) failed to materialise (non-fatal)", failures)
		return 1
	}
	return 0
}
