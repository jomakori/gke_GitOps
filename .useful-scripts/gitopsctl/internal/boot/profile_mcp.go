package boot

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Administrator-managed secret layer, merged into every profile scope.
const (
	managedScopeDir  = "/etc/hermes"
	managedScopeEnv  = managedScopeDir + "/.env"
	managedScopePerm = 0o640
)

// dataRoot is the launch HERMES_HOME (the PVC mount).
const dataRoot = "/opt/data"

// SyncProfileMCP mirrors the launch mcp_servers block into every profile config
// (pinning HERMES_HOME in stdio entries) and writes its ${VAR} values to the
// managed env, so profiles adopt the launch connections instead of re-dialing.
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

// pinLaunchHome pins HERMES_HOME into every stdio entry's env, so the connection
// identity matches across scopes. Entries that declare it are left alone.
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
				// flow-style env: add the key to the mapping
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

// inlineEnvWithHome adds HERMES_HOME to a flow-style `env: {...}` mapping.
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

// isServerEntry reports whether line is a server key (two-space indent).
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

// syncProfileSecrets writes the block's resolvable ${VAR} values to the managed
// env; variables missing from environ are skipped.
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
	// group-readable: the gateway runs as the runtime user
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

// quoteEnvValue emits a double-quoted dotenv value.
func quoteEnvValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

// blockServerNames lists the server keys of an mcp_servers block.
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

// topLevelBlock returns the block from a top-level `key:` to the next top-level key.
func topLevelBlock(text, key string) (string, bool) {
	start, end := blockBounds(text, key)
	if start < 0 {
		return "", false
	}
	return strings.Join(strings.SplitAfter(text, "\n")[start:end], ""), true
}

// replaceTopLevelBlock swaps key's block; text without the key is unchanged.
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

// blockBounds returns the start and end line indexes of key's block.
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

// isTopLevelKey reports whether line is a column-zero mapping key.
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
