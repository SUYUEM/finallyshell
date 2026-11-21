package app

import (
	"path"
	"strings"

	"finallyshell/internal/config"
	"finallyshell/internal/icons"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// sidebarRailWidth is the width of the far-left MobaXterm vertical tab rail.
const sidebarRailWidth = 46

// showSidebarPage opens the left sidebar (if collapsed) and selects page.
func (a *App) showSidebarPage(page int) {
	if a.sidebarCollapsed {
		a.setSidebarCollapsed(false)
	}
	a.setLeftPage(page)
}

// toggleSidebar collapses or expands the left sidebar body, saving the choice.
func (a *App) toggleSidebar() { a.setSidebarCollapsed(!a.sidebarCollapsed) }

// setSidebarCollapsed sets whether the left sidebar body is collapsed.
func (a *App) setSidebarCollapsed(collapsed bool) {
	if a.sidebarCollapsed == collapsed {
		return
	}
	a.sidebarCollapsed = collapsed
	a.saveLayout()
}

// sidebarRail is MobaXterm's far-left vertical rail of sidebar mode tabs
// (Sessions tree, SFTP browser, Server monitor, Quick commands/Macros), plus
// collapse and settings controls at the bottom.
func (a *App) sidebarRail(c *ui.Context) {
	type railTab struct {
		page  int
		icon  string
		label string
		tip   string
	}
	tabs := []railTab{
		{leftConnections, "server", locales.SidebarSessions(), locales.ConnectionsPage()},
		{leftSFTP, "folder", locales.SidebarSFTP(), locales.SidebarSFTP()},
		{leftMonitor, "cpu", locales.SidebarMonitor(), locales.SidebarMonitor()},
		{leftCommands, "terminal", locales.SidebarCommands(), locales.SidebarCommands()},
	}
	ui.Column(c.Key("moba-rail")).Width(sidebarRailWidth).FillHeight().Shrink(0).
		Background(colors.rail).BorderWidth(0, 1, 0, 0).BorderColor(colors.line).
		PaddingY(4).Gap(2).AlignItems(ui.Stretch).Children(func() {
		for _, t := range tabs {
			active := !a.sidebarCollapsed && a.left.page == t.page
			btn := ui.Column(c.Key("rail-"+t.icon)).Padding(7, 2).Gap(2).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Role(ui.RoleToggleButton).Checked(active).
				Label(t.tip).Tooltip(t.tip).Shrink(0)
			iconColor, textColor := theme.TextMuted, theme.TextMuted
			switch {
			case active:
				btn.Background(colors.railActive)
				iconColor, textColor = theme.Accent, theme.Accent
			case btn.Hovered():
				btn.Background(colors.railHover)
				iconColor, textColor = theme.Text, theme.Text
			}
			btn.Children(func() {
				if active {
					ui.Box(c).Absolute().Top(2).Bottom(2).Left(0).Width(3).Radius(2).Background(theme.Accent)
				}
				ui.Icon(c, icons.Get(t.icon)).FontSize(16).TextColor(iconColor)
				ui.Text(c, t.label).FontSize(9.5).SingleLine().TextColor(textColor)
			})
			if btn.Clicked() {
				if !a.sidebarCollapsed && a.left.page == t.page {
					a.setSidebarCollapsed(true)
				} else {
					a.showSidebarPage(t.page)
				}
			}
		}

		ui.Spacer(c)

		// Collapse / Expand sidebar toggle
		collapseIcon := "chevron-left"
		if a.sidebarCollapsed {
			collapseIcon = "chevron-right"
		}
		ui.Column(c).AlignItems(ui.Center).Gap(4).PaddingY(4).Children(func() {
			if widgets.IconButton(c, collapseIcon, locales.ToolbarToggleSidebar()).Size(28, 28).Clicked() {
				a.toggleSidebar()
			}
			if widgets.IconButton(c, "settings", locales.Options()).Size(28, 28).Clicked() {
				a.openOptions()
			}
		})
	})
}

// sidebarHeaderBar renders the crisp title bar at the top of each MobaXterm
// sidebar page with page-specific action buttons on the right.
func (a *App) sidebarHeaderBar(c *ui.Context, title string, actions func()) {
	ui.Row(c).Height(32).Padding(0, 6, 0, 10).Gap(2).AlignItems(ui.Center).
		Background(colors.sidebarHeader).BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Shrink(0).Children(func() {
		ui.Text(c, title).FontSize(12).Bold().SingleLine().Grow(1).MinWidth(0)
		if actions != nil {
			actions()
		}
	})
}

// mobaSidebarBody renders the active page inside the collapsible left sidebar.
func (a *App) mobaSidebarBody(c *ui.Context) {
	ui.Column(c.Key("moba-sidebar")).Fill().Background(colors.sidebar).Children(func() {
		switch a.left.page {
		case leftSFTP:
			a.sidebarSFTP(c)
		case leftMonitor:
			a.sidebarMonitor(c)
		case leftCommands:
			a.sidebarCommands(c)
		case leftSessions:
			a.sidebarOpenSessions(c)
		default:
			a.sidebarConnections(c)
		}
	})
}

// sidebarConnections is MobaXterm's User Sessions tree page in the left sidebar.
func (a *App) sidebarConnections(c *ui.Context) {
	a.sidebarHeaderBar(c, locales.ConnectionsPage(), func() {
		m := &a.connTree
		if headerButton(c, "plus", locales.NewConnection()).Clicked() {
			a.newConnectionIn(m, a.treeTarget(m))
		}
		if headerButton(c, "folder-plus", locales.NewFolder()).Clicked() {
			a.newFolderIn(m, a.treeTarget(m))
		}
		if headerButton(c, "import", locales.ImportTitle()).Clicked() {
			a.openImport()
		}
		if headerButton(c, "export", locales.ExportTitle()).Clicked() {
			a.openExport(WindowMain)
		}
		if headerButton(c, "external-link", locales.ConnectionManager()).Clicked() {
			a.openManager()
		}
	})
	ui.Column(c).Grow(1).MinHeight(0).Padding(4, 0, 0, 0).Children(func() {
		a.connectionPage(c)
	})
}

// sidebarOpenSessions is the vertical list of open sessions, kept for
// compatibility when explicitly selected.
func (a *App) sidebarOpenSessions(c *ui.Context) {
	a.sidebarHeaderBar(c, locales.SessionsTitle(a.sessionCount()), func() {
		if headerButton(c, "square-terminal", locales.LocalTerminal()).Clicked() {
			a.openLocal(nil)
		}
		if headerButton(c, "plus", locales.NewTab()).Clicked() {
			a.NewTab()
		}
	})
	a.sessionListContent(c)
}

// sidebarMonitor is MobaXterm's Server Monitor page in the left sidebar.
func (a *App) sidebarMonitor(c *ui.Context) {
	a.sidebarHeaderBar(c, locales.SidebarMonitor(), func() {
		if headerButton(c, "columns-2", a.moveMonitorLabel()).Clicked() {
			a.setMonitorOnRight(!a.monitorOnRight())
		}
	})
	if a.monitorOnRight() {
		ui.Column(c).Grow(1).Padding(16, 12).Gap(10).AlignItems(ui.Start).Children(func() {
			widgets.Muted(c, locales.MoveMonitorLeft())
			if ui.Button(c, locales.MoveMonitorLeft()).Clicked() {
				a.setMonitorOnRight(false)
			}
		})
		return
	}
	a.monitorPanel(c)
}

// sidebarSFTP is MobaXterm's signature embedded SFTP file browser in the left
// sidebar, synced with the active SSH session's s.files state.
func (a *App) sidebarSFTP(c *ui.Context) {
	s := a.connSession()
	title := locales.SidebarSFTP()
	if s != nil && !s.isNewTab() && !s.isLocal() {
		title = locales.SidebarSFTP() + " · " + s.label()
	}
	a.sidebarHeaderBar(c, title, func() {
		if s != nil && !s.isNewTab() && !s.isLocal() {
			if headerButton(c, "maximize-2", locales.SidebarOpenFullFiles()).Clicked() {
				a.openBottomTab(tabFiles)
			}
		}
	})
	if s == nil || s.isNewTab() {
		ui.Column(c).Grow(1).Center().Padding(20, 16).Gap(8).Children(func() {
			ui.Icon(c, icons.Get("folder")).FontSize(28).TextColor(theme.TextMuted)
			widgets.Muted(c, locales.SftpConnectPrompt()).FontSize(12).TextAlign(ui.Center)
		})
		return
	}
	if s.isLocal() {
		localUnavailable(c)
		return
	}
	f := &s.files
	ui.Column(c.Key(s.ID)).Grow(1).MinHeight(0).Children(func() {
		// Compact SFTP action bar (Up, Refresh, Home, Follow terminal dir, Upload, Download, New folder, Downloads)
		ui.Row(c).Padding(4, 6).Gap(2).Wrap().AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Shrink(0).Children(func() {
			if widgets.IconButton(c, "arrow-up", locales.ParentFolder()).Size(22, 22).Clicked() && f.cwd != "" {
				a.openDir(s, path.Dir(f.cwd))
			}
			if widgets.IconButton(c, "refresh-cw", locales.Refresh()).Size(22, 22).Clicked() && f.cwd != "" {
				a.openDir(s, f.cwd)
			}
			if widgets.IconButton(c, "house", locales.HomeFolder()).Size(22, 22).Clicked() && s.sftp != nil {
				if home, err := s.sftp.Getwd(); err == nil {
					a.openDir(s, home)
				}
			}
			a.followButton(c, s)
			ui.Box(c).Size(1, 14).Background(colors.line).Margin(0, 2)
			uploadMenu := func(m *ui.Menu) { a.uploadMenuItems(m, s, f.cwd) }
			if widgets.IconButton(c, "upload", locales.UploadFiles()).Size(22, 22).ContextMenu(uploadMenu).Clicked() {
				a.pickAndUpload(s, f.cwd, false)
			}
			if widgets.IconButton(c, "folder-up", locales.UploadFolder()).Size(22, 22).ContextMenu(uploadMenu).Clicked() {
				a.pickAndUpload(s, f.cwd, true)
			}
			names := f.chosenNames()
			dl := widgets.IconButton(c, "download", locales.Download()).Size(22, 22)
			dl.ContextMenu(func(m *ui.Menu) { a.downloadMenuItems(m, s, f.chosenNames()) })
			if len(names) == 0 {
				dl.Opacity(0.4)
			}
			if dl.Clicked() && len(names) > 0 {
				a.download(s, names, "")
			}
			if widgets.IconButton(c, "folder-plus", locales.NewFolder()).Size(22, 22).Clicked() && f.cwd != "" {
				a.askNew(s, f.cwd, opMkdir)
			}
			ui.Spacer(c)
			if widgets.IconButtonBadge(c, "circle-arrow-down", locales.TabTransfers(), a.transfers.active()).Size(22, 22).Clicked() {
				a.toggleDrawerTab(tabTransfers)
			}
		})

		// Path input row
		ui.Row(c).Padding(4, 6).Gap(4).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Shrink(0).Children(func() {
			if ui.TextInput(c, &f.pathEdit).Grow(1).MinWidth(0).Submitted() {
				a.openDir(s, f.pathEdit)
			}
			if f.loading {
				ui.Spinner(c)
			}
		})

		if f.err != nil {
			ui.Text(c, fileErrorText(f.err)).TextColor(theme.Danger).FontSize(11.5).Padding(4, 8)
		}

		a.sidebarFileTable(c, s)
	})
}

// sidebarFileColumns are compact columns tailored for the left SFTP sidebar.
func sidebarFileColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{ID: "name", Title: locales.ColName(), MinWidth: 110},
		{ID: "size", Title: locales.ColSize(), Width: 66, Align: ui.End},
	}
}

// sidebarFileTable renders the remote directory contents inside the left SFTP
// sidebar with drag-and-drop upload, double-click open/edit, F2 rename, Delete
// and the full file context menu.
func (a *App) sidebarFileTable(c *ui.Context, s *Session) {
	f := &s.files
	entries := f.entries
	f.sideTable.Selected = &f.row
	f.sideTable.Selection = &f.chosen
	f.sideTable.Key = func(i int) any { return entries[i].Name }
	f.sideTable.Label = func(i int) string { return entries[i].Name }
	zone := ui.Column(c).Grow(1).MinHeight(0).Background(theme.Background)
	a.dropZone(zone, s, f.cwd)
	over := zone.FileDragOver() && f.cwd != ""
	zone.Children(func() {
		t := ui.Table(c, &f.sideTable, sidebarFileColumns(), len(entries), func(row, col int) {
			e := entries[row]
			switch col {
			case 0:
				chosen := f.chosen.Has(e.Name)
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					a.entryIcon(c, e)
					name := ui.Text(c, e.Name).FontSize(12.5).SingleLine()
					if e.Broken && !chosen {
						name.TextColor(theme.Danger)
					}
				})
			case 1:
				if !e.IsDir() && !e.Broken {
					ui.Text(c, formatSize(e.Size)).FontSize(11.5)
				}
			}
		}).Grow(1)
		if t.Submitted() && f.row >= 0 && f.row < len(entries) {
			if e := entries[f.row]; e.IsDir() {
				a.openDir(s, path.Join(f.cwd, e.Name))
			} else if editable(e) {
				a.openRemoteFile(s, path.Join(f.cwd, e.Name))
			}
		}
		rename := f.sideTable.Shortcut(c, 0, ui.KeyF2)
		del := f.sideTable.Shortcut(c, 0, ui.KeyDelete)
		del = f.sideTable.Shortcut(c, ui.Cmd, ui.KeyBackspace) || del
		if e, ok := f.entry(f.chosenNames()); ok && rename {
			a.askRename(s, path.Join(f.cwd, e.Name), e.IsDir())
		}
		if del && f.chosen.Len() > 0 {
			a.askDeleteChosen(s)
		}
		t.ContextMenu(func(m *ui.Menu) {
			names := f.chosenNames()
			if e, ok := f.entry(names); ok && editable(e) {
				if m.Item(locales.Edit()).Chosen() {
					a.openRemoteFile(s, path.Join(f.cwd, e.Name))
				}
				m.Separator()
			}
			a.downloadMenuItems(m, s, names)
			m.Separator()
			a.uploadMenuItems(m, s, f.cwd)
			m.Separator()
			if m.Item(locales.NewFolder()).Chosen() {
				a.askNew(s, f.cwd, opMkdir)
			}
			if m.Item(locales.NewFile()).Chosen() {
				a.askNew(s, f.cwd, opCreate)
			}
			m.Separator()
			e, one := f.entry(names)
			if m.Item(locales.Rename()).Shortcut(0, ui.KeyF2).Disabled(!one).Chosen() && one {
				a.askRename(s, path.Join(f.cwd, e.Name), e.IsDir())
			}
			if m.Item(locales.Delete()).Shortcut(0, ui.KeyDelete).Disabled(len(names) == 0).Chosen() && len(names) > 0 {
				a.askDeleteChosen(s)
			}
			m.Separator()
			if m.Item(locales.Refresh()).Chosen() {
				a.openDir(s, f.cwd)
			}
			if len(names) == 1 && m.Item(locales.CopyPath()).Chosen() {
				c.WriteClipboard(path.Join(f.cwd, names[0]))
			}
		})
		if over {
			ui.Text(c, locales.DropToUpload(f.cwd)).Padding(6, 10).Radius(6).
				Background(theme.Accent).TextColor(theme.AccentText).
				Attach(ui.AnchorBottom, ui.AnchorBottom).Bottom(10)
		}
	})
}

// sidebarCommands is MobaXterm's compact Macros / Quick Commands browser in
// the left sidebar, sharing quickUI state with the full bottom Commands tab.
func (a *App) sidebarCommands(c *ui.Context) {
	q := &a.quickUI
	store := a.quick()
	if store.Group(q.group) == nil {
		q.group = ""
	}
	s, live := a.quickSession()

	a.sidebarHeaderBar(c, locales.SidebarCommands(), func() {
		if headerButton(c, "plus", locales.QuickNewCommand()).Clicked() {
			a.newQuickCommand()
		}
		if headerButton(c, "maximize-2", locales.SidebarOpenFullCommands()).Clicked() {
			a.openBottomTab(tabCommands)
		}
	})

	ui.Column(c.Key("sidebar-quick-commands")).Grow(1).MinHeight(0).Padding(6).Gap(6).Children(func() {
		ui.SearchField(c, &q.query).Placeholder(locales.QuickSearchShort()).Label(locales.QuickSearch()).FillWidth()

		// Compact group filter pills
		ui.Row(c).Wrap().Gap(4).Shrink(0).Children(func() {
			groupChip := func(id, name string) {
				on := q.group == id
				chip := ui.Row(c.Key("sc-grp-"+id)).Padding(2, 7).Radius(10).Cursor(ui.CursorPointer).
					Border(1, colors.line).Label(name)
				fg := theme.TextMuted
				if on {
					chip.Background(theme.Accent.Alpha(0.14)).Border(1, theme.Accent.Alpha(0.5))
					fg = theme.Accent
				} else if chip.Hovered() {
					chip.Background(colors.toolbarHover)
					fg = theme.Text
				}
				chip.Children(func() {
					ui.Text(c, name).FontSize(11).SingleLine().TextColor(fg)
				})
				if chip.Clicked() {
					q.group = id
				}
			}
			groupChip("", locales.QuickAll())
			for _, g := range store.Groups {
				groupChip(g.ID, g.Name)
			}
		})

		// Command list
		cmds := a.shownQuick(store)
		var act func()
		ui.Scroll(c).Grow(1).MinHeight(0).Gap(4).Children(func() {
			if len(cmds) == 0 {
				switch {
				case strings.TrimSpace(q.query) != "":
					widgets.Muted(c, locales.QuickNoMatches()).FontSize(12).Padding(8, 4)
				case q.group != "":
					widgets.Muted(c, locales.QuickNoneInGroup()).FontSize(12).Padding(8, 4)
				default:
					widgets.Muted(c, locales.QuickNone()).FontSize(12).Padding(8, 4)
				}
				return
			}
			for _, cmd := range cmds {
				a.sidebarCommandItem(c, s, live, cmd, &act)
			}
		})
		if act != nil {
			act()
		}
	})
}

// sidebarCommandItem renders a single compact clickable quick command card in
// the left sidebar.
func (a *App) sidebarCommandItem(c *ui.Context, s *Session, live bool, cmd *config.QuickCommand, act *func()) {
	lines := config.QuickCommandLines(cmd.Text)
	preview := ""
	if len(lines) > 0 {
		preview = strings.TrimSpace(lines[0])
	}
	card := ui.Column(c.Key("sc-cmd-"+cmd.ID)).FillWidth().Padding(6, 8).Gap(2).Radius(5).
		Background(theme.Background).Border(1, colors.line).Tooltip(cmd.Text).Label(cmd.Title())
	if live {
		card.Cursor(ui.CursorPointer)
		if card.Hovered() {
			card.Border(1, theme.Accent.Alpha(0.6)).Background(theme.Accent.Alpha(0.05))
		}
	} else {
		card.Opacity(0.55)
	}
	card.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Get("terminal")).FontSize(12).TextColor(theme.Accent).Shrink(0)
			ui.Text(c, cmd.Title()).FontSize(12).Bold().SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			if len(lines) > 1 {
				ui.Text(c, locales.QuickLines(len(lines))).FontSize(10).TextColor(theme.TextMuted).Shrink(0)
			}
		})
		if preview != "" && preview != cmd.Title() {
			ui.Text(c, preview).Font("monospace").FontSize(11).TextColor(theme.TextMuted).SingleLine().Ellipsis("…")
		}
	})
	card.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.QuickSend()).Disabled(!live).Chosen() {
			*act = func() { a.sendQuick(c, s, cmd) }
		}
		line, ok := a.barLine(s, cmd)
		if m.Item(locales.QuickPutInBar()).Disabled(!ok).Chosen() {
			*act = func() { a.putInBar(c, s, line) }
		}
		m.Separator()
		if m.Item(locales.QuickEdit()).Chosen() {
			a.editQuickCommand(cmd)
		}
		if m.Item(locales.Delete()).Danger().Chosen() {
			a.askDeleteQuick(cmd)
		}
	})
	if card.Clicked() && live {
		*act = func() { a.sendQuick(c, s, cmd) }
	}
}
