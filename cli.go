package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// bundleID is the app's identifier, as mygo.json gives it.
const bundleID = "dev.mayoi.editor"

// cliMark is a line of the mayoi command, by which uninstalling knows it.
const cliMark = "# Installed by Mayoi: File > Install 'mayoi' Command."

// cliScript is the mayoi command. It hands its folders and files to the
// app by its identifier, so it keeps working when the app moves, else to
// the app at app, where it was installed from; and returns at once: a
// running Mayoi opens them, or one starts.
func cliScript(app string) string {
	fallback := ""
	if app != "" {
		fallback = " 2>/dev/null || open -a " + shellQuote(app) + ` "$@"`
	}
	return `#!/bin/sh
# mayoi [folder or file]... opens them in Mayoi.
` + cliMark + `
for p in "$@"; do
	case $p in
	/*) ;;
	*) p=$PWD/$p ;;
	esac
	set -- "$@" "$p"
	shift
done
open -b ` + bundleID + ` "$@"` + fallback + `
`
}

// cliDirs are where the command goes: ~/.local/bin when the shell looks
// there, which needs no password, else /usr/local/bin, which is always on
// the path.
func cliDirs(home string, path []string) []string {
	local := filepath.Join(home, ".local", "bin")
	if slices.Contains(path, local) {
		return []string{local, "/usr/local/bin"}
	}
	return []string{"/usr/local/bin"}
}

// errCanceled is the user declining to give the password.
var errCanceled = errors.New("canceled")

// installCLI writes the mayoi command of the app at app into the first
// of dirs that takes it, as an administrator for the last, and returns
// where it went.
func installCLI(dirs []string, app string) (string, error) {
	var err error
	for i, dir := range dirs {
		p := filepath.Join(dir, "mayoi")
		if data, rerr := os.ReadFile(p); rerr == nil && !strings.Contains(string(data), cliMark) {
			return "", fmt.Errorf("%s is another program's", p)
		}
		if err = writeCLI(dir, app); err == nil {
			return p, nil
		}
		if i == len(dirs)-1 && errors.Is(err, os.ErrPermission) {
			if err = writeCLIAsAdmin(dir, app); err == nil {
				return p, nil
			}
		}
	}
	return "", err
}

func writeCLI(dir, app string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := filepath.Join(dir, "mayoi")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(cliScript(app)), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func writeCLIAsAdmin(dir, app string) error {
	f, err := os.CreateTemp("", "mayoi-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString(cliScript(app))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	p := filepath.Join(dir, "mayoi")
	return runAsAdmin(fmt.Sprintf("mkdir -p %s && cp %s %s && chmod 755 %s",
		shellQuote(dir), shellQuote(f.Name()), shellQuote(p), shellQuote(p)))
}

// uninstallCLI removes the mayoi command from dirs, where it is Mayoi's,
// and returns where it was.
func uninstallCLI(dirs []string) ([]string, error) {
	var removed []string
	for _, dir := range dirs {
		p := filepath.Join(dir, "mayoi")
		data, err := os.ReadFile(p)
		if err != nil || !strings.Contains(string(data), cliMark) {
			continue
		}
		err = os.Remove(p)
		if errors.Is(err, os.ErrPermission) {
			err = runAsAdmin("rm -f " + shellQuote(p))
		}
		if err != nil {
			return removed, err
		}
		removed = append(removed, p)
	}
	return removed, nil
}

// runAsAdmin runs a shell command as an administrator, which macOS asks
// the password for.
func runAsAdmin(cmd string) error {
	script := `do shell script "` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(cmd) + `" with administrator privileges`
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "(-128)") {
			return errCanceled
		}
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// appBundle is the .app the running executable is in, "" when it is in
// none.
func appBundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return ""
	}
	if app, _, ok := strings.Cut(exe, ".app/Contents/MacOS/"); ok {
		return app + ".app"
	}
	return ""
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// shellPath is where the user's shell looks for commands, which an app
// opened from the Finder does not inherit: the login shell says.
func shellPath() []string {
	path := filepath.SplitList(os.Getenv("PATH"))
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const mark = "MAYOI_PATH="
	out, _ := exec.CommandContext(ctx, sh, "-ilc", `echo "`+mark+`$PATH"`).Output()
	for line := range strings.Lines(string(out)) {
		if p, ok := strings.CutPrefix(strings.TrimSpace(line), mark); ok {
			path = append(path, filepath.SplitList(p)...)
		}
	}
	for i, p := range path {
		path[i] = filepath.Clean(p)
	}
	return path
}
