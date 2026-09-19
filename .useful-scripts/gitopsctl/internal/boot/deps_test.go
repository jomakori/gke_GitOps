package boot

import (
	"strings"
	"testing"
)

func TestDepsInstallArgs(t *testing.T) {
	const cacheDir = "/tmp/apt-cache"
	args := depsInstallArgs(cacheDir)
	t.Logf("apt-get %s", strings.Join(args, " "))

	for _, want := range []string{
		"-o",
		"Dir::Cache::Archives=" + cacheDir + "/archives",
		"Dir::State::Lists=" + cacheDir + "/lists",
		"APT::Keep-Downloaded-Packages=true",
		"install",
		"-y",
		"--no-install-recommends",
	} {
		if !contains(args, want) {
			t.Errorf("depsInstallArgs(%q) = %v, missing %q", cacheDir, args, want)
		}
	}

	for _, pkg := range strings.Fields(depsPackages) {
		if !contains(args, pkg) {
			t.Errorf("depsInstallArgs(%q) = %v, missing package %q", cacheDir, args, pkg)
		}
	}
}
