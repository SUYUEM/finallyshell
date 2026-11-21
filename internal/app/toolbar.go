package app

import (
	"strconv"

	"finallyshell/internal/icons"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// topToolbar is the MobaXterm-style top icon ribbon: grouped quick actions
// for sessions, terminal splitting, multi-execution, SFTP/monitoring/tools,
// sidebar toggling, full screen and settings.
func (a *App) topToolbar(c *ui.Context) {
	cur := a.current()
	conn := a.connSession()
	activeTunnels := 0
	if conn != nil {
		activeTunnels = conn.listening()
	}
	activeTransfers := a.transfers.active()

	ui.Row(c.Key("moba-toolbar")).Padding(4, 8).Gap(2).AlignItems(ui.Center).
		Background(colors.toolbar).BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Shrink(0).Children(func() {
		// Group 1: Sessions & Connections
		if toolbarButton(c, "server", locales.ToolbarSession(), locales.ConnectionManager(), false, false, true, 0).Clicked() {
			a.openManager()
		}
		if toolbarButton(c, "plus", locales.NewConnection(), locales.NewConnection(), false, false, false, 0).Clicked() {
			if !a.sidebarCollapsed && a.left.page == leftConnections {
				m := &a.connTree
				a.newConnectionIn(m, a.treeTarget(m))
			} else {
				a.newConnection()
			}
		}
		if toolbarButton(c, "square-terminal", locales.ToolbarLocal(), locales.LocalTerminalTip(), false, false, false, 0).Clicked() {
			a.openLocal(nil)
		}
		if toolbarButton(c, "import", locales.ImportSessions(), locales.ImportTitle(), false, false, false, 0).Clicked() {
			a.openImport()
		}
		if toolbarButton(c, "export", locales.ExportSessions(), locales.ExportTitle(), false, false, false, 0).Clicked() {
			a.openExport(WindowMain)
		}

		toolbarSep(c)

		// Group 2: Split & MultiExec
		a.toolbarSplitButton(c, cur)
		multiOn := a.sendTo.mode != targetCurrent
		if toolbarButton(c, "layout-grid", locales.ToolbarMultiExec(), locales.ToolbarMultiExecTip(), multiOn, false, false, 0).Clicked() {
			a.toggleMultiExec()
		}

		toolbarSep(c)

		// Group 3: Remote Tools (Sidebar & Bottom Drawer)
		sftpOn := !a.sidebarCollapsed && a.left.page == leftSFTP
		if toolbarButton(c, "folder", locales.ToolbarSFTP(), locales.SidebarSFTP(), sftpOn, false, false, 0).Clicked() {
			a.showSidebarPage(leftSFTP)
		}
		monOn := !a.sidebarCollapsed && a.left.page == leftMonitor
		if toolbarButton(c, "cpu", locales.ToolbarMonitor(), locales.SidebarMonitor(), monOn, false, false, 0).Clicked() {
			a.showSidebarPage(leftMonitor)
		}
		tunnelsOn := !a.bottomHidden && a.bottomTab == tabTunnels
		if toolbarButton(c, "cable", locales.TabTunnels(), locales.TabTunnels(), tunnelsOn, false, false, activeTunnels).Clicked() {
			a.toggleDrawerTab(tabTunnels)
		}
		transfersOn := !a.bottomHidden && a.bottomTab == tabTransfers
		if toolbarButton(c, "arrow-down-up", locales.TabTransfers(), locales.TabTransfers(), transfersOn, false, false, activeTransfers).Clicked() {
			a.toggleDrawerTab(tabTransfers)
		}
		procsOn := !a.bottomHidden && a.bottomTab == tabProcesses
		if toolbarButton(c, "route", locales.TabProcesses(), locales.TabProcesses(), procsOn, false, false, 0).Clicked() {
			a.toggleDrawerTab(tabProcesses)
		}
		netOn := !a.bottomHidden && a.bottomTab == tabNetwork
		if toolbarButton(c, "network", locales.TabNetwork(), locales.TabNetwork(), netOn, false, false, 0).Clicked() {
			a.toggleDrawerTab(tabNetwork)
		}
		cmdsOn := !a.sidebarCollapsed && a.left.page == leftCommands
		if toolbarButton(c, "terminal", locales.SidebarCommands(), locales.TabCommands(), cmdsOn, false, false, 0).Clicked() {
			a.showSidebarPage(leftCommands)
		}

		ui.Spacer(c)

		// Right-aligned view & settings actions
		sideIcon := "chevron-left"
		if a.sidebarCollapsed {
			sideIcon = "chevron-right"
		}
		if toolbarButton(c, sideIcon, locales.ToolbarSidebar(), locales.ToolbarToggleSidebar(), false, false, false, 0).Clicked() {
			a.toggleSidebar()
		}
		canFS := !cur.isPanelTab()
		if toolbarButton(c, "maximize-2", locales.FullScreen(), locales.FullScreen(), a.fullScreen.on, !canFS, false, 0).Clicked() && canFS {
			a.setFullScreen(!a.fullScreen.on)
		}
		if toolbarButton(c, "settings", locales.Options(), locales.Options(), false, false, false, 0).Clicked() {
			a.openOptions()
		}
	})
}

// toolbarSep is a crisp vertical divider between ribbon groups.
func toolbarSep(c *ui.Context) {
	ui.Box(c).Size(1, 30).Background(colors.line).Margin(0, 4).Shrink(0)
}

// toolbarButton renders a MobaXterm ribbon button: icon above a compact label,
// optional accent highlight, optional numeric badge, and hover/active surface.
func toolbarButton(c *ui.Context, icon, label, tip string, active, disabled, accentIcon bool, badge int) ui.Element {
	b := ui.Column(c.Key("tb-"+icon+"-"+label)).MinWidth(44).Padding(4, 7).Gap(2).
		AlignItems(ui.Center).Radius(5).Shrink(0).
		Role(ui.RoleButton).Label(label).Tooltip(tip)
	if !disabled {
		b.Cursor(ui.CursorPointer)
	}
	fg := theme.Text
	iconColor := theme.TextMuted
	if accentIcon {
		iconColor = theme.Accent
	}
	switch {
	case disabled:
		b.Opacity(0.4)
	case active:
		b.Background(colors.toolbarActive).Border(1, theme.Accent.Alpha(0.35))
		iconColor = theme.Accent
		fg = theme.Accent
	case b.Hovered():
		b.Background(colors.toolbarHover)
		if !accentIcon {
			iconColor = theme.Text
		}
	}
	if !active {
		b.Border(1, ui.Transparent)
	}
	b.Children(func() {
		ui.Box(c).Size(20, 20).Center().Children(func() {
			ui.Icon(c, icons.Get(icon)).FontSize(17).TextColor(iconColor)
			if badge > 0 {
				text := strconv.Itoa(badge)
				if badge > 99 {
					text = "99+"
				}
				ui.Text(c, text).FontSize(9).Bold().TextColor(theme.AccentText).Background(theme.Accent).
					MinWidth(13).Height(13).Radius(6.5).PaddingX(3).TextAlign(ui.Center).
					Absolute().Top(-3).Right(-6).ZIndex(1).AccessHidden(true)
			}
		})
		ui.Text(c, label).FontSize(10.5).SingleLine().TextColor(fg)
	})
	return b
}

// toolbarSplitButton is the ribbon's Split button with a dropdown popover menu
// for splitting, zooming, equalizing, moving and closing panes.
func (a *App) toolbarSplitButton(c *ui.Context, s *Session) {
	enabled := canSplit(s)
	label := locales.CmdBarSplit()
	b := ui.Column(c.Key("tb-split")).MinWidth(44).Padding(4, 7).Gap(2).
		AlignItems(ui.Center).Radius(5).Shrink(0).Border(1, ui.Transparent).
		Role(ui.RoleButton).Label(label).Tooltip(label)
	iconColor := theme.TextMuted
	switch {
	case !enabled:
		b.Opacity(0.4)
	case s.split():
		b.Background(colors.toolbarActive).Border(1, theme.Accent.Alpha(0.35))
		iconColor = theme.Accent
	case b.Hovered():
		b.Background(colors.toolbarHover)
		iconColor = theme.Text
	}
	if enabled {
		b.Cursor(ui.CursorPointer)
	}
	b.Children(func() {
		ui.Box(c).Size(20, 20).Center().Children(func() {
			ui.Icon(c, icons.Get("columns-2")).FontSize(17).TextColor(iconColor)
		})
		ui.Text(c, label).FontSize(10.5).SingleLine().TextColor(theme.Text)
	})
	if enabled {
		b.PopoverMenuButton(func(m *ui.Menu) { a.paneMenu(m, s) },
			ui.PopoverMenuButtonOptions{At: ui.AnchorBottomStart, Self: ui.AnchorTopStart})
		if s.panes.refocus {
			c.Invalidate()
		}
	}
}

// toggleMultiExec toggles MobaXterm-style multi-execution mode between the
// current session and all open SSH sessions, opening the Commands drawer when
// turned on so the user can send commands immediately.
func (a *App) toggleMultiExec() {
	if a.sendTo.mode == targetCurrent {
		a.setTarget(targetAll)
		a.openBottomTab(tabCommands)
	} else {
		a.setTarget(targetCurrent)
	}
}

// toggleDrawerTab opens the bottom tools drawer on tab, or collapses it if it
// is already open on that tab.
func (a *App) toggleDrawerTab(tab int) {
	if !a.bottomHidden && *a.shownBottomTab() == tab && !a.current().isPanelTab() {
		a.setBottomPanelHidden(true)
		return
	}
	a.openBottomTab(tab)
}
