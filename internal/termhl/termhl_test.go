package termhl

import (
	"bytes"
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

type fakeConn struct {
	chunks [][]byte
	idx    int
	wrote  bytes.Buffer
	cols   int
	rows   int
}

func (f *fakeConn) Read(p []byte) (int, error) {
	if f.idx >= len(f.chunks) {
		return 0, io.EOF
	}
	n := copy(p, f.chunks[f.idx])
	if n < len(f.chunks[f.idx]) {
		f.chunks[f.idx] = f.chunks[f.idx][n:]
	} else {
		f.idx++
	}
	return n, nil
}

func (f *fakeConn) Write(p []byte) (int, error) { return f.wrote.Write(p) }
func (f *fakeConn) Close() error                { return nil }
func (f *fakeConn) Resize(cols, rows int) error { f.cols, f.rows = cols, rows; return nil }

func readAllChunks(t *testing.T, enabled *atomic.Bool, chunks ...string) string {
	t.Helper()
	fc := &fakeConn{}
	for _, ch := range chunks {
		fc.chunks = append(fc.chunks, []byte(ch))
	}
	rw := Wrap(fc, enabled)
	got, err := io.ReadAll(rw)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

func TestHighlightMobaXtermMOTD(t *testing.T) {
	motd := " * Documentation:  https://help.ubuntu.com\r\n" +
		"System information as of Sat Nov 21 08:25:52 +0330 2026\r\n" +
		"  IPv4 address for eth0: 96.12.34.56\r\n" +
		"SSH session to root@96.12.34.56\r\n" +
		"Expanded Security Maintenance for Applications is not enabled.\r\n" +
		"Last login: Sat Nov 21 08:04:50 2026 from 74.102.248.40\r\n" +
		"root@dedirock-1013571386:~# "

	out := readAllChunks(t, nil, motd)

	for _, want := range []string{
		sgrURL + "https://help.ubuntu.com" + sgrResetURL,
		sgrInfo + "information" + sgrResetFG,
		sgrAddr + "96.12.34.56" + sgrResetFG,
		sgrAddr + "root@96.12.34.56" + sgrResetFG,
		sgrSuccess + "enabled" + sgrResetFG,
		sgrInfo + "Last login" + sgrResetFG,
		sgrAddr + "74.102.248.40" + sgrResetFG,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%q", want, out)
		}
	}
	// Plain hostname prompt should NOT be highlighted as an IP address.
	if strings.Contains(out, sgrAddr+"root@dedirock") {
		t.Errorf("unexpectedly highlighted hostname prompt:\n%q", out)
	}
}

func TestHighlightScriptKeywordsAndChinese(t *testing.T) {
	in := "[INFO] 正在下载 package...\n" +
		"[WARN] deprecated config detected, 警告: 重试\n" +
		"[ERROR] connection refused: permission denied (安装失败)\n" +
		"[OK] deployment succeeded (部署成功)\n"
	out := readAllChunks(t, nil, in)

	for _, want := range []string{
		sgrInfo + "INFO" + sgrResetFG,
		sgrInfo + "正在下载" + sgrResetFG,
		sgrWarn + "WARN" + sgrResetFG,
		sgrWarn + "deprecated" + sgrResetFG,
		sgrWarn + "警告" + sgrResetFG,
		sgrError + "ERROR" + sgrResetFG,
		sgrError + "connection refused" + sgrResetFG,
		sgrError + "permission denied" + sgrResetFG,
		sgrError + "失败" + sgrResetFG,
		sgrSuccess + "OK" + sgrResetFG,
		sgrSuccess + "succeeded" + sgrResetFG,
		sgrSuccess + "成功" + sgrResetFG,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%q", want, out)
		}
	}
}

func TestPreservesExistingColorsAndAltScreen(t *testing.T) {
	// Already colored text must NOT be overridden; reset restores highlighting.
	colored := "\x1b[33merror warning ok\x1b[0m error\n"
	out := readAllChunks(t, nil, colored)
	want := "\x1b[33merror warning ok\x1b[0m " + sgrError + "error" + sgrResetFG + "\n"
	if out != want {
		t.Errorf("colored:\n got %q\nwant %q", out, want)
	}

	// Inside alternate screen (vim/htop), everything passes through raw.
	alt := "\x1b[?1049herror 10.0.0.1 enabled\x1b[?1049lenabled"
	out = readAllChunks(t, nil, alt)
	wantAlt := "\x1b[?1049herror 10.0.0.1 enabled\x1b[?1049l" + sgrSuccess + "enabled" + sgrResetFG
	if out != wantAlt {
		t.Errorf("altScreen:\n got %q\nwant %q", out, wantAlt)
	}
}

func TestSplitEscapeAndUTF8AcrossReads(t *testing.T) {
	// Split a CSI color sequence and a Chinese keyword across Read chunks.
	zh := []byte("成功")
	out := readAllChunks(t, nil,
		"\x1b[3", "2mkeep\x1b[0", "m ",
		string(zh[:2]), string(zh[2:]),
		"\n",
	)
	want := "\x1b[32mkeep\x1b[0m " + sgrSuccess + "成功" + sgrResetFG + "\n"
	if out != want {
		t.Errorf("split:\n got %q\nwant %q", out, want)
	}
}

func TestToggleAndResize(t *testing.T) {
	var on atomic.Bool
	on.Store(false)
	out := readAllChunks(t, &on, "error 1.2.3.4 enabled\n")
	if out != "error 1.2.3.4 enabled\n" {
		t.Errorf("disabled should pass through raw, got %q", out)
	}

	fc := &fakeConn{}
	rw := Wrap(fc, &on)
	r, ok := rw.(resizer)
	if !ok {
		t.Fatal("wrapped conn lost Resize method")
	}
	if err := r.Resize(120, 40); err != nil || fc.cols != 120 || fc.rows != 40 {
		t.Errorf("resize: err=%v cols=%d rows=%d", err, fc.cols, fc.rows)
	}
}
