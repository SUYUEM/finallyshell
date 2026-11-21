package app

import (
	"testing"

	"finallyshell/internal/config"
	"finallyshell/internal/termhl"
	"finallyshell/locales"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// TestMobaXtermLayout verifies the redesigned MobaXterm UI chrome: top icon
// ribbon toolbar, far-left vertical sidebar rail (Sessions / SFTP / Monitor /
// Macros), collapsible sidebar body, horizontal session tab bar, bottom tools
// drawer (collapsed by default), and bottom status bar with live telemetry.
func TestMobaXtermLayout(t *testing.T) {
	a, p := testApp(t)
	demoSession(a, "web-1", "45.56.78.90")
	demoSession(a, "db-1", "10.0.0.12")
	a.active = 0

	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()

	// Default MobaXterm state: sidebar open on Connections tree, bottom drawer
	// collapsed, graphical bottom remote monitor bar expanded on SSH tabs.
	if a.left.page != leftConnections || a.sidebarCollapsed || !a.bottomHidden || a.bottomMonitorHidden {
		t.Fatalf("initial state: page=%d collapsed=%v bottomHidden=%v bottomMonitorHidden=%v",
			a.left.page, a.sidebarCollapsed, a.bottomHidden, a.bottomMonitorHidden)
	}
	for _, want := range []string{
		locales.ToolbarSession(),
		locales.ToolbarLocal(),
		locales.ToolbarSFTP(),
		locales.ToolbarMultiExec(),
		"web-1",
		"db-1",
		locales.SessionsTitle(2),
		"/lib64",
		"0.53 Mb/s",
		"1.01 Mb/s",
		"254ms",
	} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in MobaXterm layout; texts %q", want, tt.Texts())
		}
	}

	// Toggling the IP privacy eye button masks the IP across bottom monitor,
	// status bar, and monitor sidebar, and persists across restart.
	if err := tt.Click(locales.HideIP()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.hideIP || !config.LoadSettings().HideIP {
		t.Fatalf("hideIP not persisted: %v / %v", a.hideIP, config.LoadSettings().HideIP)
	}
	if !tt.HasText(maskedIP) || tt.HasText("45.56.78.90") {
		t.Errorf("IP not masked; texts %q", tt.Texts())
	}
	if b := New(p); !b.hideIP {
		t.Errorf("hideIP not restored on restart")
	}
	if err := tt.Click(locales.ShowIP()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.hideIP || !tt.HasText("45.56.78.90") {
		t.Errorf("IP not revealed again; hideIP=%v texts=%q", a.hideIP, tt.Texts())
	}

	// Collapsing the graphical bottom monitor bar persists and keeps speeds in the status bar.
	a.toggleBottomMonitor()
	tt.Frame()
	if !a.bottomMonitorHidden || !config.LoadSettings().BottomMonitorHidden {
		t.Fatalf("bottomMonitorHidden not persisted: %v / %v", a.bottomMonitorHidden, config.LoadSettings().BottomMonitorHidden)
	}
	if !tt.HasText("0.53 Mb/s") || !tt.HasText("1.01 Mb/s") {
		t.Errorf("collapsed status bar missing Mb/s speeds; texts %q", tt.Texts())
	}
	if b := New(p); !b.bottomMonitorHidden {
		t.Errorf("bottomMonitorHidden not restored on restart")
	}
	a.toggleBottomMonitor()
	tt.Frame()

	// Switch sidebar to embedded SFTP browser and verify remote files show inside sidebar.
	a.showSidebarPage(leftSFTP)
	tt.Frame()
	if !tt.HasText("libacl.so.1") {
		t.Errorf("embedded SFTP sidebar missing file; texts %q", tt.Texts())
	}

	// Switch sidebar to Server Monitor and verify monitor details render.
	a.showSidebarPage(leftMonitor)
	tt.Frame()
	if !tt.HasText("IP 45.56.78.90") || !tt.HasText("java") {
		t.Errorf("monitor sidebar missing stats; texts %q", tt.Texts())
	}

	// Collapse sidebar and verify persistence across restart.
	a.toggleSidebar()
	tt.Frame()
	if !a.sidebarCollapsed || !config.LoadSettings().SidebarCollapsed {
		t.Fatalf("sidebar collapse not persisted: %v / %v", a.sidebarCollapsed, config.LoadSettings().SidebarCollapsed)
	}
	if b := New(p); !b.sidebarCollapsed {
		t.Errorf("sidebarCollapsed not restored on restart")
	}

	// MultiExec toolbar toggle switches to All sessions and opens Commands drawer.
	a.toggleMultiExec()
	tt.Frame()
	if a.sendTo.mode != targetAll || a.bottomHidden || a.bottomTab != tabCommands {
		t.Errorf("MultiExec toggle: mode=%v bottomHidden=%v bottomTab=%d", a.sendTo.mode, a.bottomHidden, a.bottomTab)
	}

	// Toggling a horizontal tab selects the second session.
	if err := tt.Click("db-1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.active != 1 {
		t.Errorf("clicking db-1 tab left active=%d, want 1", a.active)
	}
}

// TestMobaClickAllBottomTabs clicks every bottom drawer tab across all sidebar
// pages on no-tab, new-tab, local-terminal, and connected SSH states to ensure
// no shared ListState or nil-session UI panics occur.
func TestMobaClickAllBottomTabs(t *testing.T) {
	modes := []string{"empty", "newtab", "local", "ssh"}
	for _, mode := range modes {
		for _, sidePage := range []int{leftConnections, leftSFTP, leftMonitor, leftCommands} {
			for _, startCollapsed := range []bool{true, false} {
				a, p := testApp(t)
				p.SetLocale("zh-Hans")
				switch mode {
				case "empty":
				case "newtab":
					a.hosts.Add(&config.Host{ID: config.NewHostID(), Name: "srv", Addr: "1.2.3.4", User: "root"})
					a.NewTab()
				case "local":
					s := newSession(nil)
					s.local = true
					s.status = statusConnected
					a.sessions = append(a.sessions, s)
					a.active = 0
				case "ssh":
					demoSession(a, "web-1", "45.56.78.90")
					a.active = 0
				}
				a.setLeftPage(sidePage)
				a.bottomHidden = startCollapsed

				tt := ui.NewTester(a.View, 1280, 820)
				tt.SetLocale("zh-Hans")
				p.tt = tt
				tt.Frame()

				// Open the bottom drawer via the status bar toggle when starting collapsed,
				// then click every actual bottom drawer tab using the Y of HidePanel.
				if a.bottomHidden {
					if err := tt.Click(locales.StatusBottomPanel()); err != nil {
						t.Fatalf("mode=%s side=%d: open drawer failed: %v", mode, sidePage, err)
					}
					tt.Frame()
					if a.bottomHidden {
						t.Fatalf("mode=%s side=%d: drawer did not open", mode, sidePage)
					}
				}
				for _, tabIdx := range []int{tabFiles, tabTransfers, tabTunnels, tabProcesses, tabNetwork, tabCommands, tabFiles} {
					btnRect, ok := tt.Find(locales.HidePanel())
					if !ok {
						t.Fatalf("mode=%s side=%d collapsed=%v: missing %q", mode, sidePage, startCollapsed, locales.HidePanel())
					}
					// In zh-Hans, each 2-char tab is ~47 DIPs wide including padding and gap.
					x := sidebarRailWidth + a.sideWidth + 24 + float32(tabIdx)*47
					tt.ClickAt(x, btnRect.Y+btnRect.H/2)
					tt.Frame()
					if a.bottomHidden {
						t.Fatalf("mode=%s side=%d startCollapsed=%v tab=%d: still hidden after click at (%v,%v)", mode, sidePage, startCollapsed, tabIdx, x, btnRect.Y+btnRect.H/2)
					}
					if a.bottomTab != tabIdx {
						t.Fatalf("mode=%s side=%d startCollapsed=%v: clicked x=%v, got tab %d want %d", mode, sidePage, startCollapsed, x, a.bottomTab, tabIdx)
					}
				}
				// Collapse again via HidePanel button
				if err := tt.Click(locales.HidePanel()); err != nil {
					t.Fatalf("mode=%s side=%d: hide failed: %v", mode, sidePage, err)
				}
				tt.Frame()
				if !a.bottomHidden {
					t.Fatalf("mode=%s side=%d: did not hide", mode, sidePage)
				}
			}
		}
	}
}

// TestMobaSyntaxHighlightTerminal renders a real Ghostty terminal fed through
// termhl.Highlight with Ubuntu MOTD and script execution output.
func TestMobaSyntaxHighlightTerminal(t *testing.T) {
	a, p := testApp(t)
	p.SetLocale("zh-Hans")
	s := demoSession(a, "Raksmart", "137.175.106.35")
	a.active = 0
	a.showSidebarPage(leftSFTP)

	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	opts := a.terminalOptions(s.Host)
	opts.Conn = idleConn{done}
	term, err := terminal.New(opts)
	if err != nil {
		t.Skip("no terminal:", err)
	}
	t.Cleanup(func() { term.Close() })
	s.setTerm(term)

	sample := "Welcome to Ubuntu 24.04.5 LTS (GNU/Linux 7.3.5-x64v2-xanmod1 x86_64)\r\n\r\n" +
		" * Documentation:  https://help.ubuntu.com\r\n" +
		" * Management:     https://landscape.canonical.com\r\n" +
		" * Support:        https://ubuntu.com/pro\r\n\r\n" +
		"System information as of Sat Nov 21 08:25:52 +0330 2026\r\n" +
		"  IPv4 address for eth0: 137.175.106.35\r\n" +
		"Expanded Security Maintenance for Applications is not enabled.\r\n" +
		"Last login: Sat Nov 21 08:04:50 2026 from 74.102.248.40\r\n" +
		"root@server:~# ./deploy.sh\r\n" +
		"[INFO] 正在下载 https://cdn.example.com/app.tar.gz ...\r\n" +
		"[WARN] deprecated flag --legacy, 警告: 重试连接 10.0.0.12:5432\r\n" +
		"[ERROR] connection refused: permission denied (旧节点连接失败)\r\n" +
		"[OK] service active & running, health check passed (部署成功!)\r\n" +
		"root@server:~# "
	term.Feed([]byte(termhl.Highlight(sample)))

	tt := ui.NewTester(a.View, 1280, 820)
	tt.SetLocale("zh-Hans")
	p.tt = tt
	tt.Frame()
	saveShot(t, tt, "moba-syntax-hl-zh.png")
}
