package app

import (
	"strconv"

	"finallyshell/internal/icons"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// tabDrag is a horizontal session tab being dragged to reorder it.
type tabDrag struct{ index int }

// sessionTabBar is the MobaXterm-style horizontal session tab bar across the
// top of the main workspace: Home button, scrollable open session tabs with
// status dots, agent icons, pane badges, inline rename, drag-reorder, close
// buttons, New Tab / Local Terminal buttons, and a quick session switcher.
func (a *App) sessionTabBar(c *ui.Context) {
	// Keep sessList.Label wired for helpers and tests that inspect tab labels.
	a.sessList.Selected = &a.active
	a.sessList.Label = func(i int) string { return listLabel(a.sessions[i]) }
	a.sessList.Reorder = a.moveSessions

	var closing *Session
	moveFrom, moveTo := -1, -1

	ui.Row(c.Key("moba-tabbar")).Height(34).Background(colors.tabStrip).
		BorderWidth(0, 0, 1, 0).BorderColor(colors.line).AlignItems(ui.Stretch).Shrink(0).Children(func() {
		// Home / Start page quick button
		homeBtn := ui.Row(c.Key("tab-home")).Padding(0, 10).AlignItems(ui.Center).
			BorderWidth(0, 1, 0, 0).BorderColor(colors.line).Cursor(ui.CursorPointer).
			Role(ui.RoleButton).Label(locales.StatusHome()).Tooltip(locales.QuickConnect()).Shrink(0)
		cur := a.current()
		homeActive := cur == nil || (cur.isNewTab() && !cur.isPanelTab())
		switch {
		case homeActive:
			homeBtn.Background(colors.tabActive)
		case homeBtn.Hovered():
			homeBtn.Background(colors.tabHover)
		}
		homeBtn.Children(func() {
			if homeActive {
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(2).Background(theme.Accent)
			}
			iconColor := theme.TextMuted
			if homeActive {
				iconColor = theme.Accent
			}
			ui.Icon(c, icons.Get("house")).FontSize(14).TextColor(iconColor)
		})
		if homeBtn.Clicked() {
			a.openOrSelectNewTab()
		}

		// Scrollable horizontal session tabs + trailing "+" / Local Terminal buttons
		ui.ScrollHorizontal(c.Key("session-tabs-scroll")).Grow(1).MinWidth(0).Scrollbars(false).
			TrackScroll(&a.sessionTabsScroll).HandleInput(a.sessionTabsWheel).Children(func() {
			ui.Row(c).FillHeight().AlignItems(ui.Stretch).Children(func() {
				for i, s := range a.sessions {
					if a.sessionTabItem(c, i, s, i == a.active, &moveFrom, &moveTo) {
						closing = s
					}
				}
				// Trailing "+" (New tab) and Local terminal buttons right after the last tab
				ui.Row(c).Padding(0, 4).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
					if widgets.IconButton(c, "plus", locales.NewTab()).Size(24, 24).Clicked() {
						a.NewTab()
					}
					if widgets.IconButton(c, "square-terminal", locales.LocalTerminal()).Size(24, 24).Clicked() {
						a.openLocal(nil)
					}
				})
			})
		})

		// Right-side session switcher pill showing open session count
		a.sessionSwitcher(c)
	})

	if moveFrom >= 0 && moveTo >= 0 && moveFrom != moveTo {
		a.moveSessions([]int{moveFrom}, moveTo)
	}
	if closing != nil {
		a.closeSession(closing)
	}
}

// openOrSelectNewTab switches to an existing new tab, or creates one.
func (a *App) openOrSelectNewTab() {
	for i, s := range a.sessions {
		if s.isNewTab() && !s.isPanelTab() {
			a.active = i
			return
		}
	}
	a.NewTab()
}

// sessionTabItem renders one horizontal tab for s at index i. It reports
// whether the user closed the tab.
func (a *App) sessionTabItem(c *ui.Context, i int, s *Session, active bool, moveFrom, moveTo *int) (closed bool) {
	renaming := a.renaming(s)
	tab := ui.Row(c.Key(s.ID)).Gap(6).Padding(0, 6, 0, 10).MinWidth(100).MaxWidth(220).
		AlignItems(ui.Center).BorderWidth(0, 1, 0, 0).BorderColor(colors.line).Shrink(0).
		Cursor(ui.CursorPointer)
	if !renaming {
		tab.Drag(tabDrag{index: i})
	}
	hovered := tab.Hovered()

	switch {
	case active:
		tab.Background(colors.tabActive)
	case hovered:
		tab.Background(colors.tabHover)
	}
	if d, over := ui.DragOver[tabDrag](tab); over && d.index != i {
		tab.Background(theme.Accent.Alpha(0.14))
	}
	if d, ok := ui.Drop[tabDrag](tab); ok && d.index != i {
		to := i
		if d.index < i {
			to = i + 1
		}
		*moveFrom, *moveTo = d.index, to
	}

	// Tooltip
	switch {
	case s.isPanelTab():
		tab.Tooltip(panelLabel(s))
	case s.name != "" && !renaming:
		tab.Tooltip(renamedTooltip(s))
	case !s.isNewTab():
		tab.Tooltip(s.target())
	}

	// Context menu
	switch {
	case s.isPanelTab():
		tab.ContextMenu(func(m *ui.Menu) {
			if m.Item(locales.BackToTerminal()).Chosen() {
				a.backToTerminal(s)
			}
			m.Separator()
			if m.Item(locales.Close()).Danger().Chosen() {
				closed = true
			}
		})
	case s.isNewTab():
		tab.ContextMenu(func(m *ui.Menu) {
			if m.Item(locales.Close()).Danger().Chosen() {
				closed = true
			}
		})
	default:
		tab.ContextMenu(func(m *ui.Menu) {
			if m.Item(locales.Reconnect()).Chosen() {
				a.reconnect(s)
			}
			if m.Item(locales.DuplicateSession()).Chosen() {
				if s.isLocal() {
					a.openLocal(nil)
				} else {
					a.openSessionIn(nil, s.Host)
				}
			}
			if m.Item(locales.Disconnect()).Disabled(s.status != statusConnected && s.status != statusConnecting).Chosen() {
				a.transfers.cancelSession(s)
				s.disconnect()
				s.status, s.err = statusClosed, nil
			}
			m.Separator()
			a.paneMenu(m, s)
			m.Separator()
			a.renameMenu(m, s)
			m.Separator()
			if m.Item(locales.Close()).Danger().Chosen() {
				closed = true
			}
		})
	}

	// Click selects; double-click on a disconnected/failed session reconnects.
	if !renaming {
		if tab.Clicked() {
			a.active = i
		}
		if tab.DoubleClicked() && !s.isNewTab() && !s.isPanelTab() && s.status != statusConnected && s.status != statusConnecting {
			a.reconnect(s)
		}
	}

	tab.Children(func() {
		// Signature MobaXterm cobalt top accent indicator on the active tab
		if active {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(2).Background(theme.Accent)
		}

		// Left icon / status dot
		switch {
		case s.isPanelTab():
			icon := "app-window"
			if s.panelTab >= 0 && s.panelTab < len(panelTabIcons) {
				icon = panelTabIcons[s.panelTab]
			}
			iconColor := theme.TextMuted
			if active {
				iconColor = theme.Accent
			}
			ui.Icon(c, icons.Get(icon)).FontSize(13).TextColor(iconColor).Shrink(0)
		case s.isNewTab():
			ui.Icon(c, icons.Get("app-window")).FontSize(13).TextColor(theme.TextMuted).Shrink(0)
		case s.isLocal():
			ui.Icon(c, icons.Get("laptop")).FontSize(13).TextColor(s.statusColor(theme)).Shrink(0)
		default:
			widgets.StatusDot(c, s.statusColor(theme))
		}

		// Title / inline rename field
		textColor := theme.TextMuted
		if active {
			textColor = theme.Text
		}
		if renaming {
			ui.Box(c).Grow(1).MinWidth(60).Children(func() {
				a.renameField(c, active)
			})
		} else {
			title := s.label()
			if s.isPanelTab() {
				title = panelLabel(s)
			}
			t := ui.Text(c, title).FontSize(12.5).SingleLine().TextColor(textColor).Grow(1).MinWidth(0)
			if active {
				t.Bold()
			}
		}

		// Coding agent icon & split pane count badge (for session tabs)
		if !s.isNewTab() && !s.isPanelTab() && !renaming {
			if ag := a.shownAgent(s); ag != nil {
				ui.Icon(c, agentIcon(ag)).FontSize(13).Shrink(0).Label(ag.Name).Tooltip(ag.Name)
			}
			if s.split() {
				tabPaneBadge(c, s)
			}
		}

		// Close button (always visible on active tab, on hover for inactive tabs)
		closeLabel := locales.CloseSession()
		if s.isPanelTab() {
			closeLabel = locales.Close()
		}
		closeBtn := widgets.IconButton(c, "x", closeLabel).Size(18, 18)
		if !hovered && !active {
			closeBtn.Invisible()
		}
		if closeBtn.Clicked() {
			closed = true
		}
	})
	return closed
}

// tabPaneBadge is a compact split-pane counter styled for horizontal tabs.
func tabPaneBadge(c *ui.Context, s *Session) {
	n := s.panes.count()
	label := locales.PaneCount(n)
	ui.Row(c.Key("panes")).Gap(2).AlignItems(ui.Center).Padding(1, 4).Radius(7).
		Background(theme.Border.Alpha(0.65)).Shrink(0).
		Label(label).Tooltip(label).Children(func() {
		ui.Icon(c, icons.Get("layout-grid")).FontSize(10).TextColor(theme.TextMuted)
		ui.Text(c, strconv.Itoa(n)).FontSize(10.5).TextColor(theme.TextMuted)
	})
}

// sessionSwitcher is a compact pill at the right end of the tab bar showing
// the open session count and opening a popover menu to jump to any tab.
func (a *App) sessionSwitcher(c *ui.Context) {
	label := locales.SessionsTitle(a.sessionCount())
	btn := ui.Row(c.Key("session-switcher")).Gap(4).Padding(0, 10).AlignItems(ui.Center).
		BorderWidth(0, 0, 0, 1).BorderColor(colors.line).Cursor(ui.CursorPointer).
		Role(ui.RoleMenuButton).Label(label).Tooltip(label).Shrink(0)
	if btn.Hovered() {
		btn.Background(colors.tabHover)
	}
	btn.Children(func() {
		ui.Text(c, label).FontSize(11.5).SingleLine().TextColor(theme.TextMuted)
		ui.Icon(c, icons.Get("chevron-down")).FontSize(12).TextColor(theme.TextMuted)
	})
	btn.PopoverMenuButton(func(m *ui.Menu) {
		if len(a.sessions) == 0 {
			m.Item(locales.NoSessions()).Disabled(true)
			return
		}
		for i, s := range a.sessions {
			if m.Item(listLabel(s)).ID("tab:" + strconv.Itoa(i)).Checked(i == a.active).Chosen() {
				a.active = i
			}
		}
		m.Separator()
		if m.Item(locales.NewTab()).Chosen() {
			a.NewTab()
		}
		if m.Item(locales.LocalTerminal()).Chosen() {
			a.openLocal(nil)
		}
	}, ui.PopoverMenuButtonOptions{At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd})
}

// sessionTabsWheel scrolls the horizontal session tab bar with the vertical
// mouse wheel when tabs overflow.
func (a *App) sessionTabsWheel(ev ui.InputEvent) bool {
	s := &a.sessionTabsScroll
	if ev.Kind != ui.InputScroll || ev.DX != 0 || ev.DY == 0 || s.MaxX <= 0 || ev.Mods&ui.Ctrl != 0 {
		return false
	}
	s.X = min(max(s.X+ev.DY, 0), s.MaxX)
	return true
}
