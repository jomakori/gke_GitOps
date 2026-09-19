package boot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// depsCacheRoot is the persistent-volume apt cache reused across pod starts.
const depsCacheRoot = "/opt/data/.apt-cache"

// depsPackages is the desktop-E2E X11 toolchain plus the Rust link-stage dev
// libraries local `cargo test` needs.
const depsPackages = "xvfb xdotool openbox imagemagick dbus-x11 bats " +
	"libwebkit2gtk-4.1-dev libgtk-3-dev libglib2.0-dev " +
	"libayatana-appindicator3-dev librsvg2-dev libxdo-dev libssl-dev"

func depsLogf(format string, args ...any) {
	fmt.Printf("deps: %s\n", fmt.Sprintf(format, args...))
}

// DepsPresent reports whether the on-demand system dependency set is installed.
func DepsPresent() bool {
	return quietOK("bats", "--version") &&
		quietOK("xdotool", "--version") &&
		quietOK("pkg-config", "--exists", "glib-2.0")
}

// depsAptOptions points apt's download caches at the persistent volume.
func depsAptOptions(cacheDir string) []string {
	return []string{
		"-o", "Dir::Cache::Archives=" + cacheDir + "/archives",
		"-o", "Dir::State::Lists=" + cacheDir + "/lists",
		"-o", "APT::Keep-Downloaded-Packages=true",
	}
}

// depsInstallArgs returns the apt-get install argv for the given cache dir.
func depsInstallArgs(cacheDir string) []string {
	args := append(depsAptOptions(cacheDir), "install", "-y", "--no-install-recommends")
	return append(args, strings.Fields(depsPackages)...)
}

// depsUpdateArgs returns the apt-get update argv for the given cache dir.
func depsUpdateArgs(cacheDir string) []string {
	return append(depsAptOptions(cacheDir), "update", "-qq")
}

// InstallDeps installs the dependency set on demand, caching apt downloads on
// the persistent volume.
func InstallDeps() error {
	for _, dir := range []string{
		filepath.Join(depsCacheRoot, "archives", "partial"),
		filepath.Join(depsCacheRoot, "lists", "partial"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	for _, args := range [][]string{
		depsUpdateArgs(depsCacheRoot),
		depsInstallArgs(depsCacheRoot),
	} {
		if err := depsRun(args); err != nil {
			return err
		}
	}
	return nil
}

// depsRun logs the exact apt-get command line before running it.
func depsRun(args []string) error {
	depsLogf("running: apt-get %s", strings.Join(args, " "))
	cmd := exec.Command("apt-get", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
