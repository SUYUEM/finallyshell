package app

import (
	"fmt"

	"finallyshell/internal/icons"
	"finallyshell/internal/monitor"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// maskedIP is shown in place of the server IP when privacy mode is enabled.
const maskedIP = "•••.•••.•••.•••"

// toggleHideIP toggles masking of server IP addresses in the main window chrome
// and persists the choice.
func (a *App) toggleHideIP() {
	a.hideIP = !a.hideIP
	a.saveLayout()
}

// displayTarget returns s.target(), masking the server address when hideIP is on.
func (a *App) displayTarget(s *Session) string {
	if !a.hideIP || s == nil || s.Host == nil {
		return s.target()
	}
	t := s.Host.User + "@" + maskedIP
	if p := s.Host.PortOr22(); p != 22 {
		t += fmt.Sprintf(":%d", p)
	}
	return t
}

// ipEyeButton renders the privacy eye toggle icon (eye / eye-off) for hiding
// or revealing the server IP address.
func (a *App) ipEyeButton(c *ui.Context, size float32) {
	icon, label := "eye", locales.HideIP()
	if a.hideIP {
		icon, label = "eye-off", locales.ShowIP()
	}
	b := widgets.ToggleIconButton(c, icon, label, a.hideIP).Size(size, size)
	if b.Clicked() {
		a.toggleHideIP()
	}
}

// toggleBottomMonitor expands or collapses the compact graphical MobaXterm
// remote monitoring strip above the status bar, saving the choice.
func (a *App) toggleBottomMonitor() {
	a.bottomMonitorHidden = !a.bottomMonitorHidden
	a.saveLayout()
}

// mobaSpeedColors returns the MobaXterm-style upload (teal) and download (blue)
// arrow colors for the current theme.
func mobaSpeedColors() (up, down ui.Color) {
	if theme.Dark {
		return ui.Hex("#2dd4bf"), ui.Hex("#60a5fa")
	}
	return ui.Hex("#0d9488"), ui.Hex("#2563eb")
}

// speedCells renders high-contrast, color-coded MobaXterm upload (teal) and
// download (cobalt blue) speed pills with bold Mb/s primary values and compact
// byte-rate secondary readouts so rates are immediately scannable at a glance.
func speedCells(c *ui.Context, tx, rx float64, showByteRate bool) {
	upCol, downCol := mobaSpeedColors()
	padY, padX, fontSize, iconSize := float32(2), float32(7), float32(12), float32(13)
	if !showByteRate {
		padY, padX, fontSize, iconSize = 1, 6, 11.5, 12
	}
	pill := func(key, icon, dirLabel string, rate float64, col ui.Color) {
		ui.Row(c.Key(key)).Padding(padY, padX).Gap(4).AlignItems(ui.Center).Radius(4).
			Background(col.Alpha(0.11)).Border(1, col.Alpha(0.32)).Shrink(0).
			Tooltip(fmt.Sprintf("%s: %s (%s/s)", dirLabel, formatMbps(rate), formatBytes(rate))).Children(func() {
			ui.Icon(c, icons.Get(icon)).FontSize(iconSize).TextColor(col).Shrink(0)
			ui.Text(c, formatMbps(rate)).FontSize(fontSize).Bold().TextColor(col).SingleLine()
			if showByteRate {
				ui.Text(c, formatBytes(rate)+"/s").FontSize(10).TextColor(theme.TextMuted).SingleLine()
			}
		})
	}
	ui.Row(c.Key("moba-speeds")).Gap(5).AlignItems(ui.Center).Shrink(0).Children(func() {
		pill("up", "arrow-up-from-line", locales.NetUpload(), tx, upCol)
		pill("down", "arrow-down-to-line", locales.NetDownload(), rx, downCol)
	})
}

// bottomMonitorBar is a low-height (66 DIP) graphical MobaXterm-style remote
// monitoring dashboard strip docked at the bottom of the window when an SSH
// session is active: Host/Uptime/Load, CPU/Mem/Swap mini gauge bars, twin
// upload/download Mb/s speed cells + live network sparkline, latency sparkline,
// disk usage bars, and clickable top processes.
func (a *App) bottomMonitorBar(c *ui.Context) {
	s := a.connSession()
	if s == nil || s.isNewTab() || s.isLocal() || a.bottomMonitorHidden {
		return
	}
	var st *monitor.Stats
	if s.status == statusConnected && execAllowed(s.Host) && s.stats.OK {
		st = &s.stats
	}

	cellSep := func() {
		ui.Box(c).Width(1).FillHeight().Background(colors.line).Shrink(0)
	}

	ui.Row(c.Key("moba-bottom-monitor")).Height(66).Background(colors.sidebar).
		BorderWidth(1, 0, 0, 0).BorderColor(colors.line).AlignItems(ui.Stretch).
		ContextMenu(a.monitorMenu).Shrink(0).Children(func() {
		// Cell 1: Full Host IP, privacy Eye toggle, Copy, System Info, Uptime & Load
		ui.Column(c).Width(212).Padding(6, 9).Gap(3).Shrink(0).Children(func() {
			ip := s.ip
			if ip == "" && s.Host != nil {
				ip = s.Host.Addr
			}
			shownIP := ip
			if a.hideIP && ip != "" {
				shownIP = maskedIP
			}
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				widgets.StatusDot(c, s.statusColor(theme))
				tip := locales.Copy() + " " + ip
				if a.hideIP {
					tip = locales.ShowIP()
				}
				ipBtn := ui.Text(c, shownIP).FontSize(11.5).Bold().SingleLine().Grow(1).MinWidth(0).
					Cursor(ui.CursorPointer).Tooltip(tip)
				if ipBtn.Clicked() && ip != "" {
					if a.hideIP {
						a.toggleHideIP()
					} else {
						c.WriteClipboard(ip)
					}
				}
				a.ipEyeButton(c, 18)
				if widgets.IconButton(c, "copy", locales.Copy()).Size(18, 18).Clicked() && ip != "" {
					c.WriteClipboard(ip)
				}
				if widgets.IconButton(c, "server", locales.SystemInfo()).Size(18, 18).Clicked() {
					a.showSidebarPage(leftMonitor)
					a.openSysInfo(s)
				}
			})
			switch {
			case s.status != statusConnected:
				widgets.Muted(c, s.statusText()).FontSize(11).SingleLine().Ellipsis("…")
			case !execAllowed(s.Host):
				widgets.Muted(c, locales.MonitorOff()).FontSize(11).SingleLine().Ellipsis("…")
			case st == nil:
				widgets.Muted(c, locales.ReadingServerInfo()).FontSize(11).SingleLine().Ellipsis("…")
			default:
				ui.Text(c, locales.UptimeDays(int(st.Uptime.Hours()/24))).FontSize(11).SingleLine()
				widgets.Muted(c, locales.LoadAverage(fmt.Sprintf("%.2f, %.2f, %.2f", st.Load[0], st.Load[1], st.Load[2]))).
					FontSize(10.5).SingleLine().Ellipsis("…")
			}
		})

		cellSep()

		// Cell 2: CPU / Memory / Swap graphical mini usage bars
		ui.Column(c).Width(178).Padding(5, 9).Gap(3).Shrink(0).Children(func() {
			if st == nil {
				widgets.MiniUsageBar(c, locales.Cpu(), 0, "0%", colors.cpu)
				widgets.MiniUsageBar(c, locales.Memory(), 0, "0%", colors.mem)
				widgets.MiniUsageBar(c, locales.Swap(), 0, "0%", colors.swap)
				return
			}
			widgets.MiniUsageBar(c, locales.Cpu(), st.CPU/100, fmt.Sprintf("%.0f%%", st.CPU), colors.cpu)
			widgets.MiniUsageBar(c, locales.Memory(), ratio(st.MemUsed, st.MemTotal), usage(st.MemUsed, st.MemTotal), colors.mem)
			widgets.MiniUsageBar(c, locales.Swap(), ratio(st.SwapUsed, st.SwapTotal), usage(st.SwapUsed, st.SwapTotal), colors.swap)
		})

		cellSep()

		// Cell 3: High-contrast Upload/Download Speeds (Mb/s + byte rate) + Live Network Sparkline
		ui.Column(c).Width(352).Padding(5, 9).Gap(4).Shrink(0).Children(func() {
			var tx, rx float64
			var txHist, rxHist []float64
			var ifLabel, ifTip string
			var ifMenu func(m *ui.Menu)
			if st != nil {
				p := a.netPicker(s)
				v := st.Net(p.chosen)
				tx, rx, txHist, rxHist = v.TxRate, v.RxRate, v.TxHist, v.RxHist
				ifLabel, ifTip = netLabel(v)
				if p.choose != nil {
					ifMenu = func(m *ui.Menu) { netMenu(m, st, v, p) }
				}
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				speedCells(c, tx, rx, true)
				ui.Spacer(c)
				if ifMenu != nil {
					netLabelButton(c, ifLabel, ifTip, ifMenu)
				} else if ifLabel != "" {
					widgets.Muted(c, ifLabel).FontSize(10.5).SingleLine()
				}
			})
			var series [][]float64
			var seriesCols []ui.Color
			if len(txHist) > 0 || len(rxHist) > 0 {
				upCol, downCol := mobaSpeedColors()
				series = [][]float64{txHist, rxHist}
				seriesCols = []ui.Color{upCol, downCol}
			}
			widgets.MiniSparkline(c, 24, monitor.HistoryLen, series, seriesCols, formatMbps)
		})

		cellSep()

		// Cell 4: Latency readout + mini sparkline
		ui.Column(c).Width(132).Padding(5, 9).Gap(4).Shrink(0).Children(func() {
			latText := "-"
			var series [][]float64
			var seriesCols []ui.Color
			if st != nil {
				latText = formatLatency(st.Latency)
				if len(st.LatHist) > 0 {
					series = [][]float64{st.LatHist}
					seriesCols = []ui.Color{latencyChart}
				}
			}
			ui.Row(c).Gap(4).AlignItems(ui.Center).Height(21).Children(func() {
				widgets.Muted(c, locales.Latency()).FontSize(11).SingleLine()
				ui.Spacer(c)
				ui.Text(c, latText).FontSize(11.5).Bold().TextColor(theme.Accent).SingleLine()
			})
			widgets.MiniSparkline(c, 26, monitor.HistoryLen, series, seriesCols, formatMillis)
		})

		cellSep()

		// Cell 5: Disk usage graphical mini bars (up to 2 primary mounts)
		ui.Column(c).Width(158).Padding(5, 9).Gap(3).Shrink(0).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				widgets.Muted(c, locales.MonitorPartDisks()).FontSize(10.5).SingleLine().Grow(1)
				widgets.Muted(c, locales.DiskFreeSize()).FontSize(10).SingleLine()
			})
			if st == nil || len(st.Disks) == 0 {
				widgets.MiniUsageBar(c, "/", 0, "-", colors.swap)
				return
			}
			for i, d := range st.Disks {
				if i >= 2 {
					break
				}
				used := uint64(0)
				if d.Size > d.Avail {
					used = d.Size - d.Avail
				}
				barCol := colors.swap
				if r := ratio(used, d.Size); r > 0.85 {
					barCol = colors.mem
				}
				widgets.MiniUsageBar(c, d.Mount, ratio(used, d.Size),
					formatBytes(float64(d.Avail))+"/"+formatBytes(float64(d.Size)), barCol)
			}
		})

		cellSep()

		// Cell 6: Top processes mini readout (clickable -> opens Processes tab)
		procCell := ui.Column(c).Grow(1).MinWidth(130).Padding(5, 9).Gap(2).
			Cursor(ui.CursorPointer).Tooltip(locales.OpenProcesses())
		if procCell.Hovered() {
			procCell.Background(colors.statusHover.Alpha(0.5))
		}
		if procCell.Clicked() {
			a.openProcesses()
		}
		procCell.Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				widgets.Muted(c, locales.Command()).FontSize(10).Grow(1).SingleLine()
				widgets.Muted(c, locales.Memory()).FontSize(10).Width(42).TextAlign(ui.End).SingleLine()
				widgets.Muted(c, locales.Cpu()).FontSize(10).Width(32).TextAlign(ui.End).SingleLine()
			})
			if st == nil || len(st.Procs) == 0 {
				widgets.Muted(c, "-").FontSize(10.5)
				return
			}
			for i, p := range st.Procs {
				if i >= 3 {
					break
				}
				ui.Row(c.Key(i)).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, p.Name).FontSize(10.5).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
					ui.Text(c, formatBytes(float64(p.RSS))).FontSize(10.5).TextColor(theme.TextMuted).Width(42).TextAlign(ui.End).SingleLine()
					ui.Text(c, fmt.Sprintf("%.1f%%", p.CPU)).FontSize(10.5).Width(32).TextAlign(ui.End).SingleLine()
				})
			}
		})
	})
}

// statusBar is MobaXterm's compact bottom bar showing the active connection's
// status and working directory, always-visible boxed upload/download speeds
// (and compact CPU/Mem/Latency when the graphical monitor strip is collapsed),
// active tunnels/transfers badges, MultiExec indicator, and panel toggles.
func (a *App) statusBar(c *ui.Context) {
	s := a.connSession()
	isSSH := s != nil && !s.isNewTab() && !s.isLocal()

	ui.Row(c.Key("moba-statusbar")).Height(26).Padding(0, 8).Gap(8).AlignItems(ui.Center).
		Background(colors.statusBar).BorderWidth(1, 0, 0, 0).BorderColor(colors.line).Shrink(0).Children(func() {
		// Left: connection status, target, and remote cwd
		ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Shrink(1).Children(func() {
			switch {
			case s == nil || s.isNewTab():
				ui.Icon(c, icons.Get("house")).FontSize(12).TextColor(theme.TextMuted)
				ui.Text(c, locales.StatusReady()).FontSize(11.5).TextColor(theme.TextMuted).SingleLine()
			case s.isLocal():
				ui.Icon(c, icons.Get("laptop")).FontSize(12).TextColor(s.statusColor(theme))
				ui.Text(c, s.label()).FontSize(11.5).SingleLine()
				if sub := s.programTitle(); sub != "" {
					widgets.Muted(c, "· "+sub).FontSize(11.5).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
				}
			default:
				widgets.StatusDot(c, s.statusColor(theme))
				ui.Text(c, s.label()).FontSize(11.5).Bold().SingleLine().Shrink(0)
				widgets.Muted(c, a.displayTarget(s)).FontSize(11.5).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
				if s.status != statusConnected {
					widgets.Muted(c, "· "+s.statusText()).FontSize(11.5).SingleLine().Shrink(0)
				}
			}
			if isSSH && s.files.cwd != "" {
				ui.Box(c).Size(1, 12).Background(colors.line).Shrink(0)
				cwdChip := ui.Row(c.Key("sb-cwd")).Gap(4).Padding(2, 6).Radius(4).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).Tooltip(locales.SidebarSFTP()).MinWidth(0).Shrink(1)
				if cwdChip.Hovered() {
					cwdChip.Background(colors.statusHover)
				}
				cwdChip.Children(func() {
					ui.Icon(c, icons.Get("folder")).FontSize(11.5).TextColor(folderColor).Shrink(0)
					ui.Text(c, s.files.cwd).Font("monospace").FontSize(11).TextColor(theme.TextMuted).
						SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
				})
				if cwdChip.Clicked() {
					a.showSidebarPage(leftSFTP)
				}
			}
		})

		ui.Spacer(c)

		// When the graphical bottom monitor strip is collapsed, keep compact
		// CPU / Memory / Latency + boxed Upload/Download speeds in the status bar.
		if isSSH && a.bottomMonitorHidden {
			var tx, rx float64
			if s.status == statusConnected && s.stats.OK {
				st := &s.stats
				v := st.Net(a.netPicker(s).chosen)
				tx, rx = v.TxRate, v.RxRate
				monChip := ui.Row(c.Key("sb-monitor")).Gap(8).Padding(2, 6).Radius(4).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).Tooltip(locales.StatusMonitorBar()).Shrink(0)
				if monChip.Hovered() {
					monChip.Background(colors.statusHover)
				}
				monChip.Children(func() {
					ui.Text(c, fmt.Sprintf("%s %.0f%%", locales.Cpu(), st.CPU)).FontSize(11).SingleLine()
					ui.Text(c, fmt.Sprintf("%s %.0f%%", locales.Memory(), 100*ratio(st.MemUsed, st.MemTotal))).FontSize(11).SingleLine()
					if st.Latency > 0 {
						ui.Text(c, formatLatency(st.Latency)).FontSize(11).TextColor(theme.Accent).SingleLine()
					}
				})
				if monChip.Clicked() {
					a.toggleBottomMonitor()
				}
			}
			speedCells(c, tx, rx, false)
		}

		// MultiExec active indicator
		if a.sendTo.mode != targetCurrent {
			chip := ui.Row(c.Key("sb-multi")).Gap(4).Padding(2, 7).Radius(4).AlignItems(ui.Center).
				Background(theme.Accent.Alpha(0.15)).Cursor(ui.CursorPointer).Tooltip(locales.ToolbarMultiExecTip()).Shrink(0)
			chip.Children(func() {
				ui.Icon(c, icons.Get("layout-grid")).FontSize(11).TextColor(theme.Accent)
				ui.Text(c, locales.ToolbarMultiExec()).FontSize(11).Bold().TextColor(theme.Accent).SingleLine()
			})
			if chip.Clicked() {
				a.openBottomTab(tabCommands)
			}
		}

		// Active tunnels indicator
		if isSSH && s.listening() > 0 {
			chip := ui.Row(c.Key("sb-tunnels")).Gap(4).Padding(2, 6).Radius(4).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Shrink(0)
			if chip.Hovered() {
				chip.Background(colors.statusHover)
			}
			chip.Children(func() {
				ui.Icon(c, icons.Get("cable")).FontSize(11.5).TextColor(theme.Accent)
				ui.Text(c, locales.TabTunnelsActive(s.listening())).FontSize(11).SingleLine()
			})
			if chip.Clicked() {
				a.toggleDrawerTab(tabTunnels)
			}
		}

		// Active transfers indicator
		if n := a.transfers.active(); n > 0 {
			chip := ui.Row(c.Key("sb-transfers")).Gap(4).Padding(2, 6).Radius(4).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Shrink(0)
			if chip.Hovered() {
				chip.Background(colors.statusHover)
			}
			chip.Children(func() {
				ui.Icon(c, icons.Get("arrow-down-up")).FontSize(11.5).TextColor(theme.Accent)
				ui.Text(c, locales.TabTransfersActive(n)).FontSize(11).SingleLine()
			})
			if chip.Clicked() {
				a.toggleDrawerTab(tabTransfers)
			}
		}

		// Toggle for the graphical bottom remote monitoring strip (SSH sessions)
		if isSSH {
			ui.Box(c).Size(1, 12).Background(colors.line).Shrink(0)
			monIcon := "chevron-down"
			if a.bottomMonitorHidden {
				monIcon = "chevron-up"
			}
			monBtn := ui.Row(c.Key("sb-mon-toggle")).Gap(4).Padding(2, 6).Radius(4).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Role(ui.RoleToggleButton).Checked(!a.bottomMonitorHidden).
				Label(locales.StatusMonitorBar()).Tooltip(locales.StatusMonitorBar()).Shrink(0)
			if !a.bottomMonitorHidden {
				monBtn.Background(theme.Accent.Alpha(0.12))
			} else if monBtn.Hovered() {
				monBtn.Background(colors.statusHover)
			}
			monColor := theme.TextMuted
			if !a.bottomMonitorHidden {
				monColor = theme.Accent
			}
			monBtn.Children(func() {
				ui.Icon(c, icons.Get(monIcon)).FontSize(12).TextColor(monColor)
				ui.Text(c, locales.StatusMonitorBar()).FontSize(11).TextColor(monColor).SingleLine()
			})
			if monBtn.Clicked() {
				a.toggleBottomMonitor()
			}
		}

		// Bottom tools drawer quick toggle
		if !a.current().isPanelTab() {
			ui.Box(c).Size(1, 12).Background(colors.line).Shrink(0)
			icon := "chevron-down"
			tip := locales.HidePanel()
			if a.bottomHidden {
				icon = "chevron-up"
				tip = locales.ShowPanel()
			}
			btn := ui.Row(c.Key("sb-drawer")).Gap(4).Padding(2, 6).Radius(4).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(locales.StatusBottomPanel()).Tooltip(tip).Shrink(0)
			if btn.Hovered() {
				btn.Background(colors.statusHover)
			}
			btn.Children(func() {
				ui.Icon(c, icons.Get(icon)).FontSize(12).TextColor(theme.TextMuted)
				ui.Text(c, locales.StatusBottomPanel()).FontSize(11).TextColor(theme.TextMuted).SingleLine()
			})
			if btn.Clicked() {
				a.toggleBottomPanel()
			}
		}
	})
}
