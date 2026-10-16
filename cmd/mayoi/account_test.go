package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/cloudsync"
	"mayoi/internal/keymap"
	"mayoi/internal/settings"
	"mayoi/locales"
)

// TestSigninDialog: the code shows until the sign-in is cancelled, by its
// button or Escape.
func TestSigninDialog(t *testing.T) {
	hs := newHarness(t, true)
	cancelled := 0
	hs.a.signin = &signin{code: "NNCD-QHTC", url: "https://mon.sorrycc.dev/device?code=NNCD-QHTC", cancel: func() { cancelled++ }}
	hs.settle()
	if !hs.tt.HasText("NNCD-QHTC") || !hs.tt.HasText(locales.SyncWaiting()) {
		t.Fatal("the code does not show")
	}
	hs.shot("signin")
	if err := hs.tt.Click(locales.ButtonCancel()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.signin != nil || cancelled != 1 || hs.tt.HasText("NNCD-QHTC") {
		t.Fatalf("Cancel: dialog %v, cancelled %d", hs.a.signin, cancelled)
	}
	hs.a.signin = &signin{code: "ABCD-EFGH", url: "https://mon.sorrycc.dev/device", cancel: func() { cancelled++ }}
	hs.settle()
	hs.tt.Key(0, ui.KeyEscape)
	hs.settle()
	if hs.a.signin != nil || cancelled != 2 {
		t.Fatalf("Escape: dialog %v, cancelled %d", hs.a.signin, cancelled)
	}
}

// TestAccountCommands: the menu offers to sign in, or names the account
// with Sync Now and Sign Out, and Sync Now signs in when signed out.
func TestAccountCommands(t *testing.T) {
	hs := newHarness(t, true)
	labels := func() []string {
		var out []string
		var walk func(items []*mygo.MenuItem)
		walk = func(items []*mygo.MenuItem) {
			for _, it := range items {
				out = append(out, it.Label)
				walk(it.Submenu)
			}
		}
		walk(hs.a.menu(false, func(string, *mygo.Window) {}).Items())
		return out
	}
	if l := labels(); !slices.Contains(l, locales.CmdAccountSignIn()) || slices.Contains(l, locales.CmdAccountSignOut()) {
		t.Errorf("signed out, the menu has %q", l)
	}
	hs.a.run("account.syncNow")
	hs.a.run("account.signOut")
	if hs.h.signIns != 1 || hs.h.syncs != 0 || hs.h.signOuts != 0 {
		t.Errorf("signed out: %d sign-ins, %d syncs, %d sign-outs", hs.h.signIns, hs.h.syncs, hs.h.signOuts)
	}
	hs.h.signedIn = "Ada"
	l := labels()
	if slices.Contains(l, locales.CmdAccountSignIn()) || !slices.Contains(l, locales.MenuSyncingAs("Ada")) || !slices.Contains(l, locales.CmdAccountSignOut()) {
		t.Errorf("signed in, the menu has %q", l)
	}
	hs.a.run("account.syncNow")
	hs.a.run("account.signOut")
	if hs.h.syncs != 1 || hs.h.signOuts != 1 {
		t.Errorf("signed in: %d syncs, %d sign-outs", hs.h.syncs, hs.h.signOuts)
	}
}

// TestCloudLocalKeybindings: applying the account's shortcuts writes
// keybindings.json, removes it for none, and keeps a conflict beside it.
func TestCloudLocalKeybindings(t *testing.T) {
	dir := t.TempDir()
	m := &mayoi{s: settings.Load(filepath.Join(dir, "settings.json"))}
	l := cloudLocal{&cloud{m: m, onMain: func(fn func()) { fn() }}}
	path := keymap.Path(m.s.Path())
	if err := l.Apply(cloudsync.Snapshot{Keybindings: "[1]"}, "[2]"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "[1]" {
		t.Errorf("keybindings.json = %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, conflictFile)); string(data) != "[2]" {
		t.Errorf("%s = %q", conflictFile, data)
	}
	s, err := l.Read()
	if err != nil || s.Keybindings != "[1]" {
		t.Errorf("read %q, %v", s.Keybindings, err)
	}
	if !l.IsDefault(cloudsync.Snapshot{Settings: m.s.Prefs.Synced(), Keybindings: keymap.Template}) {
		t.Error("the defaults are not default")
	}
	if err := l.Apply(cloudsync.Snapshot{}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("keybindings.json stayed: %v", err)
	}
}
