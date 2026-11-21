package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"finallyshell/internal/config"
)

// Finallyshell is the native Finallyshell JSON export format (and hosts.json),
// carrying folders, hosts (including proxies, jump hosts, tunnels, notes and
// terminal options), and optionally saved credentials.
const Finallyshell Kind = "finallyshell"

// FinallyshellExportFormat identifies a Finallyshell session backup JSON file.
const FinallyshellExportFormat = "finallyshell-sessions"

// ExportedSecrets holds optional credentials exported alongside a host.
type ExportedSecrets struct {
	Password      string `json:"password,omitempty"`
	Passphrase    string `json:"passphrase,omitempty"`
	ProxyPassword string `json:"proxyPassword,omitempty"`
}

func (s ExportedSecrets) empty() bool {
	return s.Password == "" && s.Passphrase == "" && s.ProxyPassword == ""
}

// FinallyshellExport is the JSON document written by ExportFinallyshell.
// Its top-level version/folders/hosts keys match hosts.json, so ReadFinallyshell
// accepts both exported backups and raw hosts.json files.
type FinallyshellExport struct {
	Format     string                     `json:"format,omitempty"`
	Version    int                        `json:"version"`
	ExportedAt time.Time                  `json:"exportedAt,omitzero"`
	Folders    []*config.Folder           `json:"folders"`
	Hosts      []*config.Host             `json:"hosts"`
	Secrets    map[string]ExportedSecrets `json:"secrets,omitempty"`
}

var finallyshellReader = &Reader{
	Kind:       Finallyshell,
	Extensions: []string{"json"},
	Read:       ReadFinallyshell,
}

// ExportFinallyshell encodes all folders and live (non-deleted) hosts from
// store into an indented JSON backup. If secrets is non-nil, non-empty entries
// for exported hosts are included under "secrets".
func ExportFinallyshell(store *config.Store, secrets map[string]ExportedSecrets, at time.Time) ([]byte, error) {
	doc := FinallyshellExport{
		Format:     FinallyshellExportFormat,
		Version:    config.HostsVersion,
		ExportedAt: at.UTC(),
		Folders:    []*config.Folder{},
		Hosts:      []*config.Host{},
	}
	if store != nil {
		if store.Version > doc.Version {
			doc.Version = store.Version
		}
		for _, f := range store.Folders {
			if f == nil {
				continue
			}
			cp := *f
			doc.Folders = append(doc.Folders, &cp)
		}
		for _, h := range store.Hosts {
			if h == nil || h.Deleted() {
				continue
			}
			cp := h.Clone()
			cp.LastConnectedAt = time.Time{}
			doc.Hosts = append(doc.Hosts, cp)
			if sec, ok := secrets[h.ID]; ok && !sec.empty() {
				if doc.Secrets == nil {
					doc.Secrets = map[string]ExportedSecrets{}
				}
				doc.Secrets[h.ID] = sec
			}
		}
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// ReadFinallyshell reads a Finallyshell exported JSON file (or hosts.json) at path.
func ReadFinallyshell(path string) (*Source, error) {
	text, err := ReadText(path)
	if err != nil {
		return nil, err
	}
	src, err := ParseFinallyshell([]byte(text), filepath.Base(path))
	if err != nil {
		return nil, err
	}
	src.Path = path
	return src, nil
}

// ParseFinallyshell decodes a Finallyshell session export JSON (or a v1/v2
// hosts.json) into a Source.
func ParseFinallyshell(data []byte, fileName string) (*Source, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, ErrNoConnections
	}
	var doc FinallyshellExport
	switch trimmed[0] {
	case '[':
		var hosts []*config.Host
		if err := json.Unmarshal(trimmed, &hosts); err != nil {
			return nil, fmt.Errorf("invalid Finallyshell JSON: %w", err)
		}
		doc.Version = 1
		doc.Hosts = hosts
	case '{':
		if err := json.Unmarshal(trimmed, &doc); err != nil {
			return nil, fmt.Errorf("invalid Finallyshell JSON: %w", err)
		}
	default:
		return nil, fmt.Errorf("not a Finallyshell JSON file")
	}

	folderByID := map[string]*config.Folder{}
	for _, f := range doc.Folders {
		if f != nil && f.ID != "" {
			folderByID[f.ID] = f
		}
	}
	folderPath := func(id string) []string {
		var parts []string
		seen := map[string]bool{}
		for id != "" && !seen[id] {
			seen[id] = true
			f := folderByID[id]
			if f == nil {
				break
			}
			if name := strings.TrimSpace(f.Name); name != "" {
				parts = append(parts, name)
			}
			id = f.Parent
		}
		slices.Reverse(parts)
		return parts
	}

	src := &Source{Kind: Finallyshell}
	byOldID := map[string]*Entry{}
	type pendingJump struct {
		entry  *Entry
		jumpID string
	}
	var jumps []pendingJump

	for i, raw := range doc.Hosts {
		if raw == nil {
			continue
		}
		at := At(fileName, i+1)
		if raw.Deleted() {
			src.Problems = append(src.Problems, NewProblem(fileName, i+1, DeletedThere, nil))
			continue
		}
		if strings.TrimSpace(raw.Addr) == "" {
			src.Problems = append(src.Problems, NewProblem(fileName, i+1, NoHost, nil))
			continue
		}
		origID := raw.ID
		origFolder := raw.Folder
		h := raw.Clone()
		h.ID = ""
		h.Folder = ""
		h.LastConnectedAt = time.Time{}
		h.DeletedAt = time.Time{}
		h.Addr = strings.TrimSpace(h.Addr)
		if h.Auth == "" {
			if h.KeyPath != "" {
				h.Auth = config.AuthKey
			} else {
				h.Auth = config.AuthPassword
			}
		}
		if h.Port < 0 || h.Port > 65535 {
			h.Port = 0
		}

		e := &Entry{
			File:   at,
			Host:   h,
			Folder: folderPath(origFolder),
		}
		if h.Auth == config.AuthKey && h.KeyPath != "" {
			e.KeyFile = h.KeyPath
		}
		if sec, ok := doc.Secrets[origID]; ok {
			switch h.Auth {
			case config.AuthKey:
				e.Password = sec.Passphrase
			case config.AuthPassword:
				e.Password = sec.Password
			}
			e.ProxyPassword = sec.ProxyPassword
		}
		if h.Proxy != nil && h.Proxy.Kind == config.ProxyJump {
			jumpID := h.Proxy.JumpHost
			h.Proxy = nil // resolved once all entries are collected
			jumps = append(jumps, pendingJump{entry: e, jumpID: jumpID})
		}

		src.Entries = append(src.Entries, e)
		if origID != "" {
			byOldID[origID] = e
		}
	}

	for _, pj := range jumps {
		if target := byOldID[pj.jumpID]; target != nil && target != pj.entry {
			pj.entry.Jump = target
		} else {
			pj.entry.Note = JumpHostNotImported
		}
	}

	if len(src.Entries) == 0 && len(src.Problems) == 0 {
		return nil, ErrNoConnections
	}
	sortSource(src)
	return src, nil
}
