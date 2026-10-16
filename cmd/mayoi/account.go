package main

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/cloudsync"
	"mayoi/internal/keymap"
	"mayoi/internal/mon"
	"mayoi/internal/settings"
	"mayoi/locales"
)

// monURL is Mon's, the account behind settings sync, or MAYOI_MON_URL's,
// as a Mon on localhost to try it with.
func monURL() string {
	return cmp.Or(os.Getenv("MAYOI_MON_URL"), "https://mon.sorrycc.dev")
}

// tokenKey is the key of Mon's access token in the system's credential
// store.
const tokenKey = "mon-token"

// conflictFile is where the account's keybindings.json goes when it
// changed here too, beside keybindings.json.
const conflictFile = "keybindings.sync-conflict.json"

// pushDelay is how long settings sync waits after a change before saving
// it to the account, so that changes made together go together.
const pushDelay = 2 * time.Second

// pullEvery is how often, at most, settings sync asks the account for
// changes as the app comes to the front.
const pullEvery = 30 * time.Second

// cloud syncs the settings and the keyboard shortcuts with the person's
// Mon account. Its fields are the main thread's, but for the engine.
type cloud struct {
	m      *mayoi
	client *mon.Client
	engine *cloudsync.Engine
	// cancelSignIn stops a sign-in waiting for its approval.
	cancelSignIn context.CancelFunc
	// asked is set once a first sync asked whose settings to keep, which
	// a sync of its own asks again only as the person syncs.
	asked    bool
	push     *time.Timer
	lastPull time.Time
	// onMain runs fn on the main thread and waits for it: mygo.RunOnMain,
	// but in tests.
	onMain func(fn func())
}

// startCloud loads the account settings sync last had, and syncs with it.
func (m *mayoi) startCloud() {
	if m.s.Path() == "" {
		return
	}
	dir := filepath.Dir(m.s.Path())
	c := &cloud{m: m, client: mon.New(monURL()), onMain: mygo.RunOnMain}
	c.engine = cloudsync.New(c.client, cloudLocal{c}, filepath.Join(dir, "sync.json"))
	m.cloud = c
	m.s.OnPrefsWritten(func() { mygo.RunOnMain(c.changed) })
	mygo.App.OnDidBecomeActive(func() {
		if time.Since(c.lastPull) > pullEvery {
			c.sync(cloudsync.Ask, false)
		}
	})
	if c.engine.State().UserID == "" {
		return
	}
	go func() {
		token, err := mygo.Secrets.Get(tokenKey)
		mygo.RunOnMain(func() {
			if errors.Is(err, mygo.ErrSecretNotFound) {
				// The token is gone: the next sign-in starts anew.
				c.engine.Forget()
				m.refreshMenu()
				return
			}
			if err != nil {
				// The keychain is locked or out of reach: sync waits for
				// the next launch.
				return
			}
			c.client.Token = string(token)
			c.sync(cloudsync.Ask, false)
		})
	}()
}

// account is the name of the person settings sync syncs for, "" for no
// one.
func (c *cloud) account() string {
	if c == nil {
		return ""
	}
	st := c.engine.State()
	if st.UserID == "" {
		return ""
	}
	return cmp.Or(st.Name, st.Email, st.UserID)
}

// changed saves this machine's settings to the account a moment after
// they change: the preferences or the shortcuts.
func (c *cloud) changed() {
	if c == nil || c.account() == "" || c.client.Token == "" {
		return
	}
	if c.push != nil {
		c.push.Stop()
	}
	c.push = time.AfterFunc(pushDelay, func() {
		res, err := c.engine.Push(context.Background())
		mygo.RunOnMain(func() { c.done(res, err, false) })
	})
}

// sync merges the account's settings with this machine's, the first time
// as choice says. manual tells of the outcome, as Sync Now does.
func (c *cloud) sync(choice cloudsync.Choice, manual bool) {
	if c.account() == "" || c.client.Token == "" {
		return
	}
	c.lastPull = time.Now()
	go func() {
		res, err := c.engine.Sync(context.Background(), choice)
		mygo.RunOnMain(func() { c.done(res, err, manual) })
	}()
}

// done tells of a sync's outcome, and asks whose settings to keep when a
// first sync needs to know.
func (c *cloud) done(res cloudsync.Result, err error, manual bool) {
	m := c.m
	switch {
	case errors.Is(err, cloudsync.ErrNeedsChoice):
		if c.asked && !manual {
			return
		}
		c.asked = true
		m.inFront(func(a *app) {
			a.confirm = &confirm{
				title: locales.SyncChooseTitle(), message: locales.SyncChooseMessage(),
				ok: locales.SyncUseAccount(), run: func() { c.sync(cloudsync.UseAccount, true) },
				alternate: locales.SyncUseLocal(), runAlternate: func() { c.sync(cloudsync.UseLocal, true) },
			}
		})
	case errors.Is(err, mon.ErrUnauthorized):
		// Signed out elsewhere, as from the account page.
		c.forget()
		m.toast(locales.SyncSessionEnded())
	case err != nil:
		// Offline, most likely: the next sync merges what this one did not.
		if manual {
			m.toast(locales.SyncFailed(err.Error()))
		}
	case res.KeybindingsConflict != "":
		m.toast(locales.SyncKeybindingsConflict())
	case manual && !res.Applied && !res.Pushed:
		m.toast(locales.SyncUpToDate())
	}
}

// syncNow syncs at once, and asks again whose settings to keep if a first
// sync waits for it.
func (c *cloud) syncNow() {
	c.sync(cloudsync.Ask, true)
}

// signIn signs in to Mon, showing the code to approve in a's window, and
// syncs once it is approved.
func (c *cloud) signIn(a *app) {
	if c.cancelSignIn != nil {
		c.cancelSignIn()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelSignIn = cancel
	host, _ := os.Hostname()
	go func() {
		// Where there is no credential store, the token has nowhere to go.
		if _, err := mygo.Secrets.Get(tokenKey); errors.Is(err, errors.ErrUnsupported) {
			mygo.RunOnMain(func() { c.m.toast(locales.SyncNoKeyring()) })
			return
		}
		dc, err := c.client.StartDeviceLogin(ctx, cloudsync.App, host)
		if err != nil {
			mygo.RunOnMain(func() { c.signInFailed(ctx, err) })
			return
		}
		mygo.RunOnMain(func() {
			if ctx.Err() != nil {
				return
			}
			a.signin = &signin{code: dc.UserCode, url: dc.VerificationURIComplete, cancel: cancel}
			a.services.OpenURL(dc.VerificationURIComplete)
			a.invalidate()
		})
		token, err := c.client.WaitForDeviceToken(ctx, dc)
		var u *mon.User
		if err == nil {
			err = mygo.Secrets.Set(tokenKey, []byte(token))
		}
		if err == nil {
			signedIn := *c.client
			signedIn.Token = token
			u, err = signedIn.Me(ctx)
		}
		mygo.RunOnMain(func() {
			if a.signin != nil && a.signin.code == dc.UserCode {
				a.signin = nil
				a.invalidate()
			}
			if err != nil {
				c.signInFailed(ctx, err)
				return
			}
			c.client.Token = token
			c.asked = false
			if err := c.engine.SignedIn(u); err != nil {
				c.signInFailed(ctx, err)
				return
			}
			c.m.toast(locales.SyncSignedIn(c.account()))
			c.m.refreshMenu()
			c.sync(cloudsync.Ask, false)
		})
	}()
}

// signInFailed says why a sign-in did not happen, unless it was
// cancelled.
func (c *cloud) signInFailed(ctx context.Context, err error) {
	switch {
	case ctx.Err() != nil:
	case errors.Is(err, mon.ErrAccessDenied):
		c.m.toast(locales.SyncDeclined())
	case errors.Is(err, mon.ErrExpired):
		c.m.toast(locales.SyncExpired())
	case errors.Is(err, errors.ErrUnsupported):
		c.m.toast(locales.SyncNoKeyring())
	default:
		c.m.toast(locales.SyncSignInFailed(err.Error()))
	}
}

// signOut stops syncing, and revokes the token on Mon.
func (c *cloud) signOut() {
	client := *c.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client.Logout(ctx)
	}()
	c.forget()
	c.m.toast(locales.SyncSignedOut())
}

// forget forgets the account and its token, as signed out.
func (c *cloud) forget() {
	if c.push != nil {
		c.push.Stop()
	}
	c.client.Token = ""
	c.engine.Forget()
	go mygo.Secrets.Delete(tokenKey)
	c.m.refreshMenu()
}

// cloudLocal is this machine's settings and shortcuts, as settings sync
// reads and writes them, from the engine's goroutine.
type cloudLocal struct{ c *cloud }

func (l cloudLocal) keysPath() string { return keymap.Path(l.c.m.s.Path()) }

func (l cloudLocal) Read() (cloudsync.Snapshot, error) {
	var s cloudsync.Snapshot
	l.c.onMain(func() { s.Settings = l.c.m.s.Prefs.Synced() })
	data, err := os.ReadFile(l.keysPath())
	if err != nil && !os.IsNotExist(err) {
		return s, err
	}
	s.Keybindings = string(data)
	return s, nil
}

func (l cloudLocal) Apply(s cloudsync.Snapshot, conflict string) error {
	var err error
	l.c.onMain(func() {
		m := l.c.m
		var p = m.s.Prefs
		if p, err = p.WithSynced(s.Settings); err != nil {
			return
		}
		m.reload(p)
		m.s.Flush()
	})
	if err != nil {
		return err
	}
	path := l.keysPath()
	if conflict != "" {
		if err := writeAtomic(filepath.Join(filepath.Dir(path), conflictFile), []byte(conflict)); err != nil {
			return err
		}
	}
	old, rerr := os.ReadFile(path)
	switch {
	case rerr == nil && string(old) == s.Keybindings:
	case s.Keybindings == "":
		if rerr == nil {
			return os.Remove(path)
		}
	default:
		return writeAtomic(path, []byte(s.Keybindings))
	}
	return nil
}

func (l cloudLocal) IsDefault(s cloudsync.Snapshot) bool {
	var p bool
	l.c.onMain(func() {
		q, err := l.c.m.s.Prefs.WithSynced(s.Settings)
		p = err == nil && q.IsDefault()
	})
	return p && (s.Keybindings == "" || s.Keybindings == keymap.Template)
}

func (l cloudLocal) Known(key string) bool { return slices.Contains(settings.PrefKeys, key) }

// writeAtomic writes data to path through a file beside it, so that a
// watcher never reads half of it.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// signin is the dialog of a sign-in to settings sync: the code to approve
// on Mon, and what cancels the sign-in.
type signin struct {
	code, url string
	cancel    func()
}

func (a *app) signinView(c *ui.Context) {
	si := a.signin
	if si == nil {
		return
	}
	p := a.pal
	cancel := func() {
		a.signin = nil
		si.cancel()
	}
	host := si.url
	if u, err := url.Parse(si.url); err == nil {
		host = u.Host
	}
	ui.Overlay(c, func() {
		back := a.dialogBack(c, "signin")
		back.Children(func() {
			box := a.floating(c, ui.Column(c).Width(420).MaxWidthPercent(92).Margin(titleBarH+8, 0, 0, 0).Padding(14, 16).Gap(10).Radius(10), true).
				Role(ui.RoleDialog).Label(locales.SyncSignInTitle())
			box.Children(func() {
				ui.Text(c, locales.SyncSignInTitle()).FontSize(13).FontWeight(600).TextColor(p.text)
				ui.Text(c, locales.SyncSignInHint(host)).FontSize(12.5).TextColor(p.muted)
				ui.Row(c).Justify(ui.Center).Padding(6, 0).Children(func() {
					ui.Text(c, si.code).Font(a.font.family).FontSize(26).FontWeight(600).LetterSpacing(2).TextColor(p.text).Selectable()
				})
				ui.Text(c, locales.SyncWaiting()).FontSize(12).TextColor(p.faint)
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, locales.ButtonCancel()).Clicked() {
						cancel()
					}
					if ui.Button(c, locales.SyncCopyCode()).Clicked() {
						a.services.WriteClipboard(si.code)
						a.tell(note{message: locales.SyncCodeCopied()})
					}
					if ui.PrimaryButton(c, locales.SyncOpenBrowser()).AutoFocus().Clicked() {
						a.services.OpenURL(si.url)
					}
				})
			})
			if back.OverlayShortcut(0, ui.KeyEscape) {
				cancel()
			}
		})
	})
}

// inFront runs fn on the app of the window focused last, before its next
// frame.
func (m *mayoi) inFront(fn func(a *app)) {
	if len(m.wins) == 0 {
		return
	}
	h := m.wins[0]
	h.do(func() { fn(h.a) })
}

// toast tells of msg in the window focused last.
func (m *mayoi) toast(msg string) {
	m.inFront(func(a *app) { a.tell(note{message: msg}) })
}

// The host's side of settings sync, which the app's commands run.

func (h *desktop) account() string { return h.m.cloud.account() }

func (h *desktop) signIn() {
	if h.m.cloud != nil {
		h.m.cloud.signIn(h.a)
	}
}

func (h *desktop) signOut() {
	if h.m.cloud != nil {
		h.m.cloud.signOut()
	}
}

func (h *desktop) syncNow() {
	if h.m.cloud != nil {
		h.m.cloud.syncNow()
	}
}
