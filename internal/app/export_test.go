package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"finallyshell/internal/config"
	"finallyshell/internal/importer"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

func TestExportAndImportAllSessionsUI(t *testing.T) {
	// 1. Set up source App with nested folders, jump host, tunnels, and saved passwords.
	a1, p1 := testApp(t)
	fCloud := a1.hosts.AddFolder("Cloud", "")
	fProd := a1.hosts.AddFolder("Prod", fCloud.ID)
	bastion := &config.Host{
		ID:     "h1",
		Name:   "Bastion",
		Addr:   "203.0.113.10",
		Port:   2222,
		User:   "admin",
		Auth:   config.AuthPassword,
		Folder: fCloud.ID,
	}
	appSrv := &config.Host{
		ID:     "h2",
		Name:   "AppServer",
		Addr:   "198.51.100.42",
		Port:   22,
		User:   "deploy",
		Auth:   config.AuthPassword,
		Folder: fProd.ID,
		Proxy:  &config.ProxyConfig{Kind: config.ProxyJump, JumpHost: bastion.ID},
		Tunnels: []config.Tunnel{
			{Kind: config.TunnelLocal, ListenPort: 9090, TargetHost: "127.0.0.1", TargetPort: 9090},
		},
	}
	a1.hosts.Hosts = []*config.Host{bastion, appSrv}
	_ = p1.SecretSet(hostSecretKey(bastion.ID, secretPassword), "bastion-pw")
	_ = p1.SecretSet(hostSecretKey(appSrv.ID, secretPassword), "app-pw")

	outDir := t.TempDir()
	outWithSecrets := filepath.Join(outDir, "backup-with-pw.json")
	outSafe := filepath.Join(outDir, "backup-safe.json")

	// Export WITHOUT passwords first from main window.
	mainTT := ui.NewTester(a1.View, 1280, 800)
	mainTT.Frame()
	p1.pickSave(outSafe, nil)
	if err := mainTT.Click(locales.ExportSessions()); err != nil {
		t.Fatalf("click top export: %v; texts %q", err, mainTT.Texts())
	}
	mainTT.Frame()
	if a1.exp == nil {
		t.Fatal("export modal did not open from top toolbar")
	}
	if err := mainTT.Click(locales.ExportAction()); err != nil {
		t.Fatalf("click export action: %v", err)
	}
	p1.until(t, "safe export done", func() bool { return a1.exp != nil && !a1.exp.busy && a1.exp.saved == outSafe })
	mainTT.Frame()
	safeData, err := os.ReadFile(outSafe)
	if err != nil {
		t.Fatalf("read safe export: %v", err)
	}
	if strings.Contains(string(safeData), "bastion-pw") || strings.Contains(string(safeData), "app-pw") {
		t.Fatal("safe export leaked passwords")
	}
	if err := mainTT.Click(locales.Close()); err != nil {
		t.Fatalf("close export modal: %v", err)
	}
	mainTT.Frame()
	if a1.exp != nil {
		t.Fatal("export modal did not close")
	}

	// Export WITH passwords from manager window.
	a1.openManager()
	mgrTT := p1.show(t, WindowManager)
	p1.pickSave(outWithSecrets, nil)
	if err := mgrTT.Click(locales.Export()); err != nil {
		t.Fatalf("click manager export: %v; texts %q", err, mgrTT.Texts())
	}
	mgrTT.Frame()
	if a1.exp == nil || a1.exp.win != WindowManager {
		t.Fatalf("manager export modal state = %+v", a1.exp)
	}
	if err := mgrTT.Click(locales.ExportIncludePasswords()); err != nil {
		t.Fatalf("click include passwords: %v", err)
	}
	mgrTT.Frame()
	if !a1.exp.includePasswords {
		t.Fatal("includePasswords checkbox did not toggle")
	}
	if err := mgrTT.Click(locales.ExportAction()); err != nil {
		t.Fatalf("click export action with passwords: %v", err)
	}
	p1.until(t, "secret export done", func() bool { return a1.exp != nil && !a1.exp.busy && a1.exp.saved == outWithSecrets })
	mgrTT.Frame()

	// 2. Import into a brand new empty App via the Import window!
	a2, p2 := testApp(t)
	a2.importKind = importer.Finallyshell
	p2.pickTyped(outWithSecrets)
	_, impTT := openImport(t, a2, p2)
	p2.until(t, "finallyshell backup read", func() bool { return !a2.imp.busy })
	impTT.Frame()

	if a2.imp.source.Kind != importer.Finallyshell {
		t.Fatalf("import source = %s", a2.imp.source.Kind)
	}
	if len(a2.imp.rows) != 2 {
		t.Fatalf("import rows = %d, want 2", len(a2.imp.rows))
	}
	if !impTT.HasText("Bastion") || !impTT.HasText("AppServer") || !impTT.HasText("••••") {
		t.Fatalf("expected hosts and masked password column in import table; texts: %q", impTT.Texts())
	}

	if err := impTT.Click(locales.ImportCount(2)); err != nil {
		t.Fatalf("click import: %v", err)
	}
	p2.until(t, "secrets stored", func() bool { return a2.secrets.queue.idle() })

	if len(a2.hosts.Live()) != 2 {
		t.Fatalf("imported %d hosts, want 2", len(a2.hosts.Live()))
	}
	var gotBastion, gotApp *config.Host
	for _, h := range a2.hosts.Live() {
		switch h.Name {
		case "Bastion":
			gotBastion = h
		case "AppServer":
			gotApp = h
		}
	}
	if gotBastion == nil || gotApp == nil {
		t.Fatalf("missing imported hosts: %+v", a2.hosts.Live())
	}
	if gotApp.Proxy == nil || gotApp.Proxy.Kind != config.ProxyJump || gotApp.Proxy.JumpHost != gotBastion.ID {
		t.Errorf("jump host not restored: %+v (bastion %s)", gotApp.Proxy, gotBastion.ID)
	}
	if len(gotApp.Tunnels) != 1 || gotApp.Tunnels[0].ListenPort != 9090 {
		t.Errorf("tunnels not restored: %+v", gotApp.Tunnels)
	}
	p2.until(t, "bastion password stored", func() bool {
		return p2.secrets()[hostSecretKey(gotBastion.ID, secretPassword)] == "bastion-pw"
	})
	p2.until(t, "app password stored", func() bool {
		return p2.secrets()[hostSecretKey(gotApp.ID, secretPassword)] == "app-pw"
	})
}
