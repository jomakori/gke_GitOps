package boot

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Where Hermes looks for administrator-managed config. The managed `.env` layer
// is merged into every profile's secret scope with override, which is the only
// supported way to give a profile-scoped session a credential the launch
// process holds in its own environment (see hermes_cli/managed_scope.py).
const (
	managedScopeDir  = "/etc/hermes"
	managedScopeEnv  = managedScopeDir + "/.env"
	managedScopePerm = 0o640
)

// dataRoot is the launch process's HERMES_HOME — the PVC mount. Every profile
// home is a directory under it, and it is the one value a profile scope
// resolves differently from the launch scope.
const dataRoot = "/opt/data"

// The launch scope is the single source of truth for the MCP manifest:
// values.yaml renders it into config.yaml, and gitopsctl mcp prewarm/verify
// certify exactly those entries. Per-profile configs, however, are runtime
// state on the PVC — Hermes copies the launch config when a profile is created
// and never refreshes it, so a profile keeps whatever mcp_servers existed on
// the day it was made.
//
// A multiplexed gateway (dashboard/webapp mode) runs one MCP discovery per
// profile home at startup, in series. A stale block is dialled from scratch
// every boot: entries that no longer exist fail, entries whose launch twin
// gained an env mapping fail differently, and entries that are lazy in the
// manifest are still eager here because the old block predates the flag. Each
// profile that cannot settle inside its connect timeout blocks the next one,
// which turned a ~15s boot into minutes of Argo CD `Progressing`.
//
// SyncProfileMCP makes every profile home a mirror of the launch scope:
//
//  1. config.yaml's top-level mcp_servers block becomes the launch block, so
//     profiles name the same servers with the same command, args, env keys and
//     timeouts, plus one addition: every stdio entry's env pins HERMES_HOME to
//     the launch home. Hermes resolves that key per scope (`_build_safe_env`
//     sets it to the ACTIVE home, so a profile would compute its own path), and
//     an explicit value in the entry's own env wins. Pinning it on the profile
//     side is what makes the connection identity — the digest adoption
//     compares — equal to the launch scope's; the launch block itself stays
//     exactly what the chart rendered.
//  2. /etc/hermes/.env receives every `${VAR}` the block references that exists
//     in the boot environment. Profile scopes never read os.environ on a miss
//     (fail-closed multiplexing), so without this layer a mirrored block renders
//     empty credentials in profile scope, the child dies with `Connection
//     closed`, and the block is worse than the stale one it replaced.
//
// With both in place `register_connected_into_current_scope` adopts the launch
// scope's live connections before any candidate is selected, so a profile pass
// registers the already-connected servers and their tools instead of spawning
// a private copy of every MCP server in the manifest.
//
// It is idempotent: unchanged profiles are not rewritten, and the pre-sync
// config is kept once as config.yaml.bak-mcp-sync.
func SyncProfileMCP() {
	launchConfig := filepath.Join(dataRoot, "config.yaml")
	profiles := filepath.Join(dataRoot, "profiles")
	changed, err := syncProfileMCP(launchConfig, profiles)
	if err != nil {
		logf("profile mcp sync: %v", err)
	} else {
		for _, path := range changed {
			logf("profile mcp sync: %s mirrors the launch manifest", path)
		}
	}
	written, mirrored, referenced, err := syncProfileSecrets(launchConfig, managedScopeEnv, os.Environ())
	if err != nil {
		logf("profile secret scope: %v", err)
	} else if written {
		logf("profile secret scope: mirrored %d of %d launch MCP variables into %s", mirrored, referenced, managedScopeEnv)
	}
}

func syncProfileMCP(launchConfigPath, profilesRoot string) ([]string, error) {
	launchConfig, err := os.ReadFile(launchConfigPath)
	if err != nil {
		return nil, fmt.Errorf("read launch config: %w", err)
	}
	block, ok := topLevelBlock(string(launchConfig), "mcp_servers")
	if !ok {
		return nil, fmt.Errorf("launch config has no mcp_servers block")
	}
	if len(blockServerNames(block)) == 0 {
		return nil, fmt.Errorf("launch config has an empty mcp_servers block")
	}
	block = pinLaunchHome(block)

	entries, err := os.ReadDir(profilesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read profiles: %w", err)
	}

	var changed []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(profilesRoot, entry.Name(), "config.yaml")
		original, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		updated, replaced := replaceTopLevelBlock(string(original), "mcp_servers", block)
		if !replaced || updated == string(original) {
			continue
		}
		mode := os.FileMode(0o600)
		if fi, err := os.Stat(path); err == nil {
			mode = fi.Mode().Perm()
		}
		backup := path + ".bak-mcp-sync"
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			_ = os.WriteFile(backup, original, mode)
			_ = os.Chown(backup, runtimeUID, runtimeUID)
		}
		if err := os.WriteFile(path, []byte(updated), mode); err != nil {
			return changed, fmt.Errorf("write %s: %w", path, err)
		}
		_ = os.Chown(path, runtimeUID, runtimeUID)
		changed = append(changed, path)
	}
	return changed, nil
}

// pinLaunchHome returns block with HERMES_HOME pinned into every stdio entry's
// env. Hermes' stdio launch resolves the child env as "safe baseline, then
// HERMES_HOME = the ACTIVE home, then the entry's own env", so a profile scope
// otherwise resolves a different value from the launch scope and the two
// connections fail the identity comparison that adoption depends on. An entry
// that already declares the key is left alone: it resolves the same value in
// both scopes anyway.
func pinLaunchHome(block string) string {
	lines := strings.SplitAfter(block, "\n")
	pin := "      HERMES_HOME: \"" + dataRoot + "\"\n"
	insert := map[int]string{}  // text emitted right after a line
	replace := map[int]string{} // line rewritten in place

	for i := 0; i < len(lines); {
		if !isServerEntry(lines[i]) {
			i++
			continue
		}
		end := i + 1
		for end < len(lines) && !isServerEntry(lines[end]) {
			end++
		}
		if entryIsStdio(lines[i:end]) && !entryDeclaresHome(lines[i:end]) {
			envLine, inlineEnv, commandLine := -1, -1, -1
			for j := i + 1; j < end; j++ {
				switch {
				case strings.HasPrefix(lines[j], "    command:") && commandLine < 0:
					commandLine = j
				case strings.HasPrefix(lines[j], "    env:"):
					if strings.TrimSpace(lines[j]) == "env:" {
						envLine = j
					} else if inlineEnv < 0 {
						inlineEnv = j
					}
				}
			}
			switch {
			case envLine >= 0:
				insert[envLine] = pin
			case inlineEnv >= 0:
				// `env: {}` or `env: {A: b}` — add the key to the flow mapping.
				replace[inlineEnv] = inlineEnvWithHome(lines[inlineEnv])
			case commandLine >= 0:
				insert[commandLine] = "    env:\n" + pin
			}
		}
		i = end
	}

	var b strings.Builder
	b.Grow(len(block) + len(insert)*len(pin))
	for i, line := range lines {
		if rewritten, ok := replace[i]; ok {
			b.WriteString(rewritten)
		} else {
			b.WriteString(line)
		}
		if extra, ok := insert[i]; ok {
			b.WriteString(extra)
		}
	}
	return b.String()
}

// inlineEnvWithHome rewrites a flow-style `env:` mapping so it declares
// HERMES_HOME. Only a manifest that hand-rolls `env: {KEY: value}` reaches this.
func inlineEnvWithHome(line string) string {
	trimmed := strings.TrimSuffix(line, "\n")
	open := strings.Index(trimmed, "{")
	close := strings.LastIndex(trimmed, "}")
	if open < 0 || close < open {
		return line
	}
	inner := strings.TrimSpace(trimmed[open+1 : close])
	entry := "HERMES_HOME: \"" + dataRoot + "\""
	if inner != "" {
		entry = inner + ", " + entry
	}
	return trimmed[:open] + "{" + entry + "}" + strings.TrimSuffix(trimmed[close+1:], "\n") + "\n"
}

// isServerEntry reports whether line opens an mcp_servers entry: a mapping key
// at exactly two spaces of indent.
func isServerEntry(line string) bool {
	if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "   ") {
		return false
	}
	body := strings.TrimSuffix(line, "\n")
	if strings.HasPrefix(body[2:], "#") || strings.TrimSpace(body) == "" {
		return false
	}
	key, _, found := strings.Cut(body[2:], ":")
	return found && key != "" && !strings.ContainsAny(key, " \t")
}

func entryIsStdio(entry []string) bool {
	for _, line := range entry[1:] {
		if strings.HasPrefix(line, "    command:") {
			return true
		}
	}
	return false
}

func entryDeclaresHome(entry []string) bool {
	for _, line := range entry[1:] {
		if strings.HasPrefix(line, "      HERMES_HOME:") || strings.HasPrefix(line, "      HERMES_HOME :") {
			return true
		}
	}
	return false
}

var placeholderPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// syncProfileSecrets writes the launch block's `${VAR}` references that the boot
// environment can resolve into the managed `.env` layer. Variables the pod does
// not have are skipped: a profile then fails that one server exactly as the
// launch scope would, and the log names the count so a missing secret is
// visible instead of silent.
func syncProfileSecrets(launchConfigPath, envPath string, environ []string) (written bool, mirrored, referenced int, err error) {
	launchConfig, err := os.ReadFile(launchConfigPath)
	if err != nil {
		return false, 0, 0, fmt.Errorf("read launch config: %w", err)
	}
	block, ok := topLevelBlock(string(launchConfig), "mcp_servers")
	if !ok {
		return false, 0, 0, fmt.Errorf("launch config has no mcp_servers block")
	}
	names := placeholders(block)
	referenced = len(names)
	if referenced == 0 {
		return false, 0, 0, nil
	}

	values := make(map[string]string, len(environ))
	for _, entry := range environ {
		if key, value, ok := strings.Cut(entry, "="); ok {
			values[key] = value
		}
	}

	var body strings.Builder
	for _, name := range names {
		value, ok := values[name]
		if !ok || value == "" {
			continue
		}
		body.WriteString(name)
		body.WriteString("=")
		body.WriteString(quoteEnvValue(value))
		body.WriteString("\n")
		mirrored++
	}
	if mirrored == 0 {
		return false, 0, referenced, nil
	}

	next := []byte(body.String())
	if current, err := os.ReadFile(envPath); err == nil && string(current) == string(next) {
		return false, mirrored, referenced, nil
	}
	if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
		return false, mirrored, referenced, fmt.Errorf("create managed scope dir: %w", err)
	}
	tmp := envPath + ".tmp"
	if err := os.WriteFile(tmp, next, managedScopePerm); err != nil {
		return false, mirrored, referenced, fmt.Errorf("write managed scope: %w", err)
	}
	// The gateway runs as the runtime user, not as root: the file has to be
	// group-readable or the managed scope silently parses as empty.
	_ = os.Chown(tmp, 0, runtimeUID)
	if err := os.Rename(tmp, envPath); err != nil {
		return false, mirrored, referenced, fmt.Errorf("replace managed scope: %w", err)
	}
	_ = os.Chmod(envPath, managedScopePerm)
	return true, mirrored, referenced, nil
}

// placeholders lists the `${VAR}` names a block references, in a stable order.
func placeholders(block string) []string {
	seen := map[string]bool{}
	var names []string
	for _, match := range placeholderPattern.FindAllStringSubmatch(block, -1) {
		if seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		names = append(names, match[1])
	}
	sort.Strings(names)
	return names
}

// quoteEnvValue emits a dotenv double-quoted value. Hermes' tokenizer reverses
// `\"` and `\\` inside double quotes and strips a trailing `# ...` only when the
// value is unquoted, so quoting is what keeps a value with spaces or a `#` intact.
func quoteEnvValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

// blockServerNames lists the server names (the two-space-indented keys) of an
// mcp_servers block.
func blockServerNames(block string) map[string]bool {
	names := map[string]bool{}
	for _, line := range strings.Split(block, "\n") {
		if !isServerEntry(line) {
			continue
		}
		names[strings.TrimSuffix(strings.TrimPrefix(line, "  "), ":")] = true
	}
	return names
}

// topLevelBlock returns the block that starts at a top-level `key:` line and
// ends before the next top-level key (or EOF), including the trailing newline.
func topLevelBlock(text, key string) (string, bool) {
	start, end := blockBounds(text, key)
	if start < 0 {
		return "", false
	}
	return strings.Join(strings.SplitAfter(text, "\n")[start:end], ""), true
}

// replaceTopLevelBlock swaps the block for key with block. A file without the
// key is returned unchanged: profiles that never carried the manifest are left
// alone rather than having a block transplanted into an unknown position.
func replaceTopLevelBlock(text, key, block string) (string, bool) {
	start, end := blockBounds(text, key)
	if start < 0 {
		return text, false
	}
	lines := strings.SplitAfter(text, "\n")
	var b strings.Builder
	b.WriteString(strings.Join(lines[:start], ""))
	b.WriteString(block)
	b.WriteString(strings.Join(lines[end:], ""))
	return b.String(), true
}

// blockBounds locates the line index where key's block starts and where the next
// top-level key begins (or the line count at EOF).
func blockBounds(text, key string) (int, int) {
	lines := strings.SplitAfter(text, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSuffix(line, "\n") == key+":" {
			start = i
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	for i := start + 1; i < len(lines); i++ {
		if isTopLevelKey(lines[i]) {
			return start, i
		}
	}
	return start, len(lines)
}

// isTopLevelKey reports whether line is a mapping key at column zero. YAML
// block scalars are indented, so a column-zero `word:` cannot be a value
// continuation of the block being replaced.
func isTopLevelKey(line string) bool {
	line = strings.TrimSuffix(line, "\n")
	colon := strings.IndexByte(line, ':')
	if colon <= 0 {
		return false
	}
	for _, r := range line[:colon] {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.':
		default:
			return false
		}
	}
	return true
}
