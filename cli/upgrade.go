// pygo upgrade — Updates the PyGo CLI to the latest version from GitHub.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const repo = "PyGo-Labs/pygo-framework"

func runUpgrade(args []string) error {
	// Resolve target version
	target := "latest"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		target = args[0]
	}

	fmt.Printf("🔄 PyGo upgrade\n\n")

	// Get current version
	currentVersion := version
	fmt.Printf("  Current: v%s\n", currentVersion)

	// If not in git repo, use release download
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		return upgradeFromRelease(target)
	}

	// Git repo: fetch latest
	fmt.Println("  Fetching latest from origin...")
	if err := exec.Command("git", "fetch", "origin").Run(); err != nil {
		return fmt.Errorf("git fetch failed: %v", err)
	}

	// Compare commits
	current, _ := exec.Command("git", "rev-parse", "HEAD").Output()
	latest, _ := exec.Command("git", "rev-parse", "origin/main").Output()

	if string(current) == string(latest) {
		fmt.Println("✅ Already up to date!")
		return nil
	}

	fmt.Println("  New changes available, pulling...")
	if err := exec.Command("git", "pull", "origin", "main").Run(); err != nil {
		return fmt.Errorf("git pull failed: %v", err)
	}

	fmt.Println("  Rebuilding CLI...")
	if err := exec.Command("go", "build", "-o", binPath(), "./cli/").Run(); err != nil {
		return fmt.Errorf("build failed: %v", err)
	}

	fmt.Println("✅ PyGo upgraded!")
	return nil
}

func upgradeFromRelease(target string) error {
	os_ := runtime.GOOS
	arch := runtime.GOARCH

	if target == "latest" {
		fmt.Println("  Resolving latest release...")
		out, err := exec.Command("curl", "-fsSL",
			fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)).Output()
		if err != nil {
			return fmt.Errorf("resolve latest: %v", err)
		}
		var release struct {
			TagName string `json:"tag_name"`
		}
		json.Unmarshal(out, &release)
		target = release.TagName
	}

	fmt.Printf("  Downloading %s for %s-%s...\n", target, os_, arch)

	filename := fmt.Sprintf("pygo-%s-%s-%s", strings.TrimPrefix(target, "v"), os_, arch)
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, target, filename)

	tmp, err := os.CreateTemp("", "pygo-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if err := exec.Command("curl", "-fsSL", url, "-o", tmp.Name()).Run(); err != nil {
		return fmt.Errorf("download failed: %v", err)
	}

	binPath := binPath()
	os.Rename(tmp.Name(), binPath)
	os.Chmod(binPath, 0o755)

	fmt.Printf("✅ PyGo %s installed at %s\n", target, binPath)
	return nil
}

func binPath() string {
	exe, err := os.Executable()
	if err == nil {
		return exe
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "bin", "pygo")
}
