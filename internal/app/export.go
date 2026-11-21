package app

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/icons"
	"finallyshell/internal/importer"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// exportState is the state of the Export All Sessions modal dialog.
type exportState struct {
	win              WindowKind
	includePasswords bool
	busy             bool
	saved            string
	savedCount       int
	err              error
}

// openExport opens the Export All Sessions dialog over win (WindowMain or WindowManager).
func (a *App) openExport(win WindowKind) {
	if win == "" {
		win = WindowMain
	}
	a.exp = &exportState{win: win}
	a.redraw()
}

// exportDialog renders the Export All Sessions modal when opened on win.
func (a *App) exportDialog(c *ui.Context, win WindowKind) {
	st := a.exp
	if st == nil || st.win != win {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		a.exportModalView(c, st)
	})
	if !open && !st.busy {
		a.exp = nil
	}
}

func (a *App) exportModalView(c *ui.Context, st *exportState) {
	live := a.hosts.Live()
	hostCount := len(live)
	folderCount := len(a.hosts.Folders)

	ui.Column(c).Width(460).Gap(12).Padding(18, 20).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Get("export")).FontSize(18).TextColor(theme.Accent)
			ui.Text(c, locales.ExportTitle()).FontSize(15).Bold()
		})

		switch {
		case hostCount == 0:
			widgets.Muted(c, locales.ExportEmpty())
		case folderCount > 0:
			ui.Text(c, locales.ExportSummary(hostCount, folderCount))
		default:
			ui.Text(c, locales.ExportSummaryNoFolders(hostCount))
		}

		if hostCount > 0 && st.saved == "" {
			ui.Column(c).Gap(6).Padding(10, 12).Radius(6).
				Background(theme.Surface).Border(1, theme.Border).Children(func() {
				ui.Checkbox(c, &st.includePasswords, locales.ExportIncludePasswords()).Disabled(st.busy)
				if st.includePasswords {
					ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
						ui.Icon(c, icons.Get("triangle-alert")).FontSize(14).TextColor(theme.Danger).Margin(2, 0, 0, 0).Shrink(0)
						ui.Text(c, locales.ExportPasswordsWarning()).FontSize(12).TextColor(theme.Danger).Grow(1).MinWidth(0)
					})
				} else {
					widgets.Muted(c, locales.ExportPasswordsNote()).FontSize(12)
				}
			})
		}

		if st.busy {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				widgets.Muted(c, locales.Exporting())
			})
		}
		if st.err != nil {
			ui.Text(c, locales.ExportFailed(st.err.Error())).TextColor(theme.Danger).FontSize(12)
		}
		if st.saved != "" {
			display := tildePath(st.saved, a.home(), runtime.GOOS)
			ui.Column(c).Gap(4).Padding(10, 12).Radius(6).
				Background(theme.Surface).Border(1, theme.Accent.Alpha(0.35)).Children(func() {
				ui.Text(c, locales.ExportDone(st.savedCount, display)).FontSize(12.5).Tooltip(st.saved)
			})
		}

		ui.Row(c).Gap(8).Justify(ui.End).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
			if st.saved != "" {
				if ui.Button(c, locales.ShowInFolder()).Clicked() {
					a.plat.ShowInFolder(st.saved)
				}
				if ui.PrimaryButton(c, locales.Close()).Clicked() {
					a.exp = nil
				}
				return
			}
			if ui.Button(c, locales.Cancel()).Disabled(st.busy).Clicked() {
				a.exp = nil
			}
			if ui.PrimaryButton(c, locales.ExportAction()).Disabled(hostCount == 0 || st.busy).Clicked() {
				a.startExport(st)
			}
		})
	})
}

// startExport snapshots the live hosts and folders, optionally gathers saved
// secrets from memory and the system credential store, prompts for a save path
// off the UI thread, and writes the Finallyshell JSON backup file.
func (a *App) startExport(st *exportState) {
	if st.busy {
		return
	}
	snapshot := &config.Store{Version: a.hosts.Version}
	for _, f := range a.hosts.Folders {
		if f != nil {
			cp := *f
			snapshot.Folders = append(snapshot.Folders, &cp)
		}
	}
	for _, h := range a.hosts.Live() {
		snapshot.Hosts = append(snapshot.Hosts, h.Clone())
	}
	if len(snapshot.Hosts) == 0 {
		return
	}

	st.busy, st.err, st.saved = true, nil, ""
	includePasswords := st.includePasswords
	win := st.win
	title := locales.ExportSaveTitle()
	typeName := locales.FinallyshellFileTypes()
	now := time.Now()
	defaultName := "finallyshell-sessions-" + now.Format("2006-01-02") + ".json"
	startPath := filepath.Join(a.plat.DownloadsDir(), defaultName)

	// Collect secrets already known in memory for the snapshot hosts.
	memSecrets := map[string]importer.ExportedSecrets{}
	type secretQuery struct {
		hostID string
		kind   string
		key    string
	}
	var queries []secretQuery
	if includePasswords {
		proxyMem := a.proxySecrets()
		for _, h := range snapshot.Hosts {
			var sec importer.ExportedSecrets
			if v := a.passwords[h.ID]; v != "" {
				switch h.Auth {
				case config.AuthKey:
					sec.Passphrase = v
				case config.AuthPassword:
					sec.Password = v
				}
			} else if h.Auth != config.AuthAgent && h.RemembersPassword() && !a.secretsUnavailable() {
				if h.Auth == config.AuthKey {
					queries = append(queries, secretQuery{hostID: h.ID, kind: secretPassphrase, key: hostSecretKey(h.ID, secretPassphrase)})
				} else {
					queries = append(queries, secretQuery{hostID: h.ID, kind: secretPassword, key: hostSecretKey(h.ID, secretPassword)})
				}
			}
			if usesProxyPassword(h) {
				if pv := proxyMem[proxySecretKey(h.ID)]; pv != "" {
					sec.ProxyPassword = pv
				} else if h.RemembersPassword() && !a.secretsUnavailable() {
					queries = append(queries, secretQuery{hostID: h.ID, kind: secretProxyPassword, key: hostSecretKey(h.ID, secretProxyPassword)})
				}
			}
			if sec.Password != "" || sec.Passphrase != "" || sec.ProxyPassword != "" {
				memSecrets[h.ID] = sec
			}
		}
	}

	writeFile := func(secrets map[string]importer.ExportedSecrets) {
		go func() {
			path, err := a.plat.SaveFile(win, title, startPath, typeName, []string{"json"})
			if path != "" && err == nil {
				var data []byte
				data, err = importer.ExportFinallyshell(snapshot, secrets, now)
				if err == nil {
					mode := os.FileMode(0o644)
					if includePasswords {
						mode = 0o600
					}
					err = os.WriteFile(path, data, mode)
				}
			}
			a.post(func() {
				if a.exp != st {
					return
				}
				st.busy = false
				if path == "" && err == nil { // user canceled
					return
				}
				if err != nil {
					st.err = err
					return
				}
				st.saved = path
				st.savedCount = len(snapshot.Hosts)
				count := st.savedCount
				display := tildePath(path, a.home(), runtime.GOOS)
				a.manager.notice = func() string { return locales.ExportDone(count, display) }
			})
		}()
	}

	if !includePasswords || len(queries) == 0 {
		if includePasswords {
			writeFile(memSecrets)
		} else {
			writeFile(nil)
		}
		return
	}

	// Read remaining saved secrets off the UI thread on the secret queue.
	a.secrets.queue.do(func() {
		var failed error
		for _, q := range queries {
			v, ok, err := a.plat.SecretGet(q.key)
			if err != nil {
				failed = err
				break
			}
			if !ok || v == "" {
				continue
			}
			sec := memSecrets[q.hostID]
			switch q.kind {
			case secretPassword:
				sec.Password = v
			case secretPassphrase:
				sec.Passphrase = v
			case secretProxyPassword:
				sec.ProxyPassword = v
			}
			memSecrets[q.hostID] = sec
		}
		a.post(func() {
			a.noteStore(failed)
			if a.exp != st {
				return
			}
			writeFile(memSecrets)
		})
	})
}
