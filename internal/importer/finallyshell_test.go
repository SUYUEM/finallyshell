package importer

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"finallyshell/internal/config"
)

func TestFinallyshellExportAndImportRoundTrip(t *testing.T) {
	store := &config.Store{Version: config.HostsVersion}
	fProd := store.AddFolder("Production", "")
	fWeb := store.AddFolder("Web", fProd.ID)
	fDB := store.AddFolder("DB", fProd.ID)

	bastion := &config.Host{
		ID:      "h-bastion",
		Name:    "Bastion",
		Addr:    "203.0.113.1",
		Port:    2222,
		User:    "ops",
		Auth:    config.AuthKey,
		KeyPath: "~/.ssh/id_ed25519",
		Folder:  fProd.ID,
		Notes:   "Entry jump host",
	}
	web := &config.Host{
		ID:        "h-web",
		Name:      "Web-01",
		Addr:      "198.51.100.10",
		Port:      22,
		User:      "root",
		Auth:      config.AuthPassword,
		Folder:    fWeb.ID,
		TermType:  "xterm-256color",
		NoMonitor: true,
		Proxy:     &config.ProxyConfig{Kind: config.ProxyJump, JumpHost: bastion.ID},
		Tunnels: []config.Tunnel{
			{Kind: config.TunnelLocal, ListenPort: 8080, TargetHost: "127.0.0.1", TargetPort: 80},
		},
		MonitorIfaces: []string{"eth0"},
	}
	db := &config.Host{
		ID:     "h-db",
		Name:   "DB-01",
		Addr:   "198.51.100.20",
		Port:   2200,
		User:   "pg",
		Auth:   config.AuthPassword,
		Folder: fDB.ID,
		Proxy:  &config.ProxyConfig{Kind: config.ProxySOCKS5, Addr: "127.0.0.1", Port: 1080, User: "proxyuser"},
	}
	deleted := &config.Host{
		ID:        "h-del",
		Name:      "Removed",
		Addr:      "192.0.2.99",
		User:      "root",
		Auth:      config.AuthPassword,
		DeletedAt: time.Now(),
	}
	store.Hosts = []*config.Host{bastion, web, db, deleted}

	secrets := map[string]ExportedSecrets{
		bastion.ID: {Passphrase: "key-pass"},
		web.ID:     {Password: "web-secret"},
		db.ID:      {Password: "db-secret", ProxyPassword: "socks-secret"},
		deleted.ID: {Password: "should-not-export"},
	}

	now := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	data, err := ExportFinallyshell(store, secrets, now)
	if err != nil {
		t.Fatalf("ExportFinallyshell: %v", err)
	}
	if strings.Contains(string(data), "should-not-export") || strings.Contains(string(data), "192.0.2.99") {
		t.Fatalf("deleted host leaked into export:\n%s", data)
	}

	path := filepath.Join(t.TempDir(), "sessions.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	src, err := ReadFinallyshell(path)
	if err != nil {
		t.Fatalf("ReadFinallyshell: %v", err)
	}
	if src.Kind != Finallyshell || len(src.Entries) != 3 || len(src.Problems) != 0 {
		t.Fatalf("read kind=%s entries=%d problems=%+v", src.Kind, len(src.Entries), src.Problems)
	}

	// Import into a brand new empty store.
	dest := &config.Store{}
	added := Import(dest, src.Entries, now)
	if len(added) != 3 {
		t.Fatalf("imported %d hosts, want 3", len(added))
	}

	byName := map[string]*config.Host{}
	for _, h := range added {
		byName[h.Name] = h
	}

	gotBastion := byName["Bastion"]
	gotWeb := byName["Web-01"]
	gotDB := byName["DB-01"]
	if gotBastion == nil || gotWeb == nil || gotDB == nil {
		t.Fatalf("missing hosts: %+v", byName)
	}

	// Nested folders restored cleanly.
	folderNames := func(h *config.Host) []string {
		var out []string
		for _, f := range dest.FolderPath(h.Folder) {
			out = append(out, f.Name)
		}
		return out
	}
	if !slices.Equal(folderNames(gotBastion), []string{"Production"}) {
		t.Errorf("bastion folder %v", folderNames(gotBastion))
	}
	if !slices.Equal(folderNames(gotWeb), []string{"Production", "Web"}) {
		t.Errorf("web folder %v", folderNames(gotWeb))
	}
	if !slices.Equal(folderNames(gotDB), []string{"Production", "DB"}) {
		t.Errorf("db folder %v", folderNames(gotDB))
	}

	// Jump host remapped to the new bastion ID.
	if gotWeb.Proxy == nil || gotWeb.Proxy.Kind != config.ProxyJump || gotWeb.Proxy.JumpHost != gotBastion.ID {
		t.Errorf("web proxy = %+v, want jump to %s", gotWeb.Proxy, gotBastion.ID)
	}
	// Tunnels, flags, and SOCKS5 proxy preserved.
	if len(gotWeb.Tunnels) != 1 || gotWeb.Tunnels[0].ListenPort != 8080 || !gotWeb.NoMonitor {
		t.Errorf("web settings lost: tunnels=%+v noMonitor=%v", gotWeb.Tunnels, gotWeb.NoMonitor)
	}
	if gotDB.Proxy == nil || gotDB.Proxy.Kind != config.ProxySOCKS5 || gotDB.Proxy.Port != 1080 || gotDB.Proxy.User != "proxyuser" {
		t.Errorf("db proxy = %+v", gotDB.Proxy)
	}
	if gotBastion.Auth != config.AuthKey || gotBastion.KeyPath != "~/.ssh/id_ed25519" {
		t.Errorf("bastion key auth = %s %q", gotBastion.Auth, gotBastion.KeyPath)
	}

	// Verify entry secrets.
	byEntry := map[string]*Entry{}
	for _, e := range src.Entries {
		byEntry[e.Host.Name] = e
	}
	if byEntry["Bastion"].Password != "key-pass" {
		t.Errorf("bastion passphrase = %q", byEntry["Bastion"].Password)
	}
	if byEntry["Web-01"].Password != "web-secret" {
		t.Errorf("web password = %q", byEntry["Web-01"].Password)
	}
	if byEntry["DB-01"].Password != "db-secret" || byEntry["DB-01"].ProxyPassword != "socks-secret" {
		t.Errorf("db secrets = %q / %q", byEntry["DB-01"].Password, byEntry["DB-01"].ProxyPassword)
	}
}

func TestFinallyshellExportWithoutSecrets(t *testing.T) {
	store := &config.Store{
		Hosts: []*config.Host{
			{ID: "h1", Name: "Alpha", Addr: "198.51.100.1", User: "root", Auth: config.AuthPassword},
		},
	}
	data, err := ExportFinallyshell(store, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"secrets"`) {
		t.Errorf("expected secrets key to be omitted:\n%s", data)
	}
}

func TestParseFinallyshellRawHostsV1AndCycle(t *testing.T) {
	// Version 1 bare array hosts.json
	v1 := `[{"id":"1","name":"Legacy","addr":"198.51.100.5","user":"ubuntu","auth":"password"}]`
	src, err := ParseFinallyshell([]byte(v1), "hosts.json")
	if err != nil || len(src.Entries) != 1 || src.Entries[0].Host.Name != "Legacy" {
		t.Fatalf("v1 parse: src=%+v err=%v", src, err)
	}

	// Folder cycle must not hang.
	cycle := `{
		"version": 2,
		"folders": [
			{"id": "f1", "name": "A", "parent": "f2"},
			{"id": "f2", "name": "B", "parent": "f1"}
		],
		"hosts": [
			{"id": "h1", "name": "Node", "addr": "198.51.100.9", "user": "root", "folder": "f1"}
		]
	}`
	src, err = ParseFinallyshell([]byte(cycle), "cycle.json")
	if err != nil || len(src.Entries) != 1 {
		t.Fatalf("cycle parse: src=%+v err=%v", src, err)
	}

	if r := ReaderOf(Finallyshell); r == nil || r.Folder || !slices.Equal(r.Extensions, []string{"json"}) {
		t.Errorf("reader %+v", r)
	}
}
