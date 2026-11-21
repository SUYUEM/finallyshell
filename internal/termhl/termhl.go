// Package termhl adds MobaXterm-style automatic keyword syntax highlighting to
// uncolored terminal output (IPs, URLs, success/error/warning/info keywords,
// and common script status messages).
//
// It parses the VT/ANSI byte stream carefully so that:
//   - existing ANSI escape sequences (CSI, OSC, DCS, charset) pass through untouched,
//   - text already colored by the remote program keeps its original colors,
//   - full-screen alternate-screen programs (vim, htop, less, tmux) are left completely raw,
//   - interactive single-character echo flushes with zero buffering delay, and
//   - incomplete escape sequences or UTF-8 runes split across Read calls never corrupt output.
package termhl

import (
	"bytes"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

// ANSI SGR sequences injected around matched tokens. We reset only the
// foreground (39) and underline (24) attributes so bold/background styles stay intact.
const (
	sgrResetFG    = "\x1b[39m"
	sgrResetURL   = "\x1b[24;39m"
	sgrURL        = "\x1b[4;96m" // underlined bright cyan
	sgrAddr       = "\x1b[95m"   // bright magenta (IPv4, user@IP, MAC)
	sgrSuccess    = "\x1b[92m"   // bright green
	sgrError      = "\x1b[91m"   // bright red
	sgrWarn       = "\x1b[93m"   // bright yellow
	sgrInfo       = "\x1b[96m"   // bright cyan
	maxEscBufSize = 4096
)

// tokenRule matches MobaXterm-style syntax highlights in a single left-to-right
// scan. Order matters: URLs and addresses take precedence over plain words, and
// multi-word phrases precede single-word keywords.
var tokenRe = regexp.MustCompile(
	// 1: URL (http / https)
	`(?P<url>https?://[^\s'"<>()\[\]{}]+)` +
		// 2: Optional user@ + IPv4 (+ optional :port or /cidr)
		`|(?P<ip>(?:[A-Za-z0-9_.-]+@)?(?:25[0-5]|2[0-4]\d|1?\d?\d)(?:\.(?:25[0-5]|2[0-4]\d|1?\d?\d)){3}(?::\d{1,5}|/\d{1,2})?\b)` +
		// 3: MAC address
		`|(?P<mac>\b(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}\b)` +
		// 4: Error phrases & keywords (English + Chinese)
		`|(?P<err>(?i:\b(?:permission\s+denied|access\s+denied|connection\s+refused|command\s+not\s+found|no\s+such\s+file|not\s+found|timed\s+out|segmentation\s+fault|out\s+of\s+memory|error|errors|err|fail|failed|failing|failure|fatal|critical|crit|emerg|panic|denied|refused|rejected|invalid|disabled|stopped|inactive|dead|down|false|timeout|exception|traceback|abort|aborted|crashed|corrupt|corrupted|unauthorized|forbidden|killed|segfault|offline|unreachable|broken|conflict)\b)|✗|✘|失败|错误|异常|拒绝|超时|已停止|不可用|崩溃|未找到|无权限)` +
		// 5: Success phrases & keywords (English + Chinese)
		`|(?P<ok>(?i:\b(?:ok|success|succeeded|successful|successfully|pass|passed|enabled|active|running|started|ready|healthy|connected|online|valid|verified|true|done|complete|completed|finished|listening|accepted|allowed|established|mounted|loaded|installed|updated|upgraded|created|synced)\b)|✓|✔|成功|已完成|完成|已启用|运行中|正常|通过|已连接)` +
		// 6: Warning keywords (English + Chinese)
		`|(?P<warn>(?i:\b(?:warn|warning|warnings|caution|deprecated|alert|retry|retrying|pending|waiting|slow|mismatch|missing|unhealthy|degraded|skipped|expired|throttled|unstable|incomplete|insecure)\b)|警告|注意|已过期|重试|跳过|等待中)` +
		// 7: Info / system keywords (English + Chinese)
		`|(?P<info>(?i:\b(?:last\s+login|info|information|notice|debug|help|tip|hint|note|website|checking|loading|downloading|uploading|installing|starting|restarting|reloading|building|compiling|testing|deploying|connecting)\b)|信息|提示|正在下载|正在安装|正在启动|正在连接|正在检查)`,
)

var (
	idxURL  = tokenRe.SubexpIndex("url")
	idxIP   = tokenRe.SubexpIndex("ip")
	idxMAC  = tokenRe.SubexpIndex("mac")
	idxErr  = tokenRe.SubexpIndex("err")
	idxOK   = tokenRe.SubexpIndex("ok")
	idxWarn = tokenRe.SubexpIndex("warn")
	idxInfo = tokenRe.SubexpIndex("info")
)

type resizer interface{ Resize(cols, rows int) error }

// Wrap wraps rw so uncolored terminal output receives MobaXterm-style keyword
// highlighting whenever enabled is nil or enabled.Load() is true.
// A Resize(cols, rows int) error method of rw is preserved.
func Wrap(rw io.ReadWriteCloser, enabled *atomic.Bool) io.ReadWriteCloser {
	c := &conn{
		rw:      rw,
		enabled: enabled,
		raw:     make([]byte, 4096),
	}
	if r, ok := rw.(resizer); ok {
		return &resizeConn{c, r}
	}
	return c
}

type resizeConn struct {
	*conn
	r resizer
}

func (c *resizeConn) Resize(cols, rows int) error { return c.r.Resize(cols, rows) }

type parseState int

const (
	stateNormal parseState = iota
	stateESC
	stateCSI
	stateOSC
	stateOSCEsc
	stateString    // DCS / SOS / PM / APC
	stateStringEsc // saw ESC inside DCS/SOS/PM/APC
	stateCharset   // ESC ( / ) / * / + / # expecting 1 final byte
)

type conn struct {
	rw      io.ReadWriteCloser
	enabled *atomic.Bool
	raw     []byte

	out    []byte
	state  parseState
	escBuf []byte
	utfBuf []byte

	altScreen bool
	fgStyled  bool
	bgStyled  bool
}

func (c *conn) Write(p []byte) (int, error) { return c.rw.Write(p) }
func (c *conn) Close() error                { return c.rw.Close() }

func (c *conn) isEnabled() bool {
	return c.enabled == nil || c.enabled.Load()
}

func (c *conn) Read(p []byte) (int, error) {
	if len(c.out) > 0 {
		n := copy(p, c.out)
		c.out = c.out[n:]
		return n, nil
	}
	for {
		n, err := c.rw.Read(c.raw)
		if n > 0 {
			c.process(c.raw[:n])
		}
		if err != nil {
			// Flush any partial buffers on EOF/error so nothing is lost.
			if len(c.utfBuf) > 0 {
				c.out = append(c.out, c.utfBuf...)
				c.utfBuf = c.utfBuf[:0]
			}
			if len(c.escBuf) > 0 {
				c.out = append(c.out, c.escBuf...)
				c.escBuf = c.escBuf[:0]
				c.state = stateNormal
			}
			if len(c.out) > 0 {
				m := copy(p, c.out)
				c.out = c.out[m:]
				return m, nil
			}
			return 0, err
		}
		if len(c.out) > 0 {
			m := copy(p, c.out)
			c.out = c.out[m:]
			return m, nil
		}
	}
}

// process feeds chunk through the VT/ANSI parser, highlighting only complete
// uncolored plain-text runs on the primary screen buffer.
func (c *conn) process(chunk []byte) {
	var plain []byte
	flushPlain := func() {
		if len(plain) == 0 {
			return
		}
		if c.isEnabled() && !c.altScreen && !c.fgStyled && !c.bgStyled {
			c.out = appendHighlighted(c.out, plain)
		} else {
			c.out = append(c.out, plain...)
		}
		plain = plain[:0]
	}

	// Prepend any incomplete UTF-8 lead bytes carried over from the last Read.
	data := chunk
	if len(c.utfBuf) > 0 && c.state == stateNormal {
		data = append(c.utfBuf, chunk...)
		c.utfBuf = c.utfBuf[:0]
	}

	for i := 0; i < len(data); i++ {
		b := data[i]
		switch c.state {
		case stateNormal:
			if b == 0x1b {
				flushPlain()
				c.escBuf = append(c.escBuf[:0], b)
				c.state = stateESC
				continue
			}
			// Keep control characters (\r, \n, \b, \x07, etc.) as hard token
			// boundaries so words never merge across lines or carriage returns.
			if b < 0x20 && b != '\t' {
				flushPlain()
				c.out = append(c.out, b)
				continue
			}
			// Check for an incomplete multi-byte UTF-8 sequence at the very end
			// of this Read chunk; hold at most 3 bytes for the next Read.
			if b >= 0x80 && len(data)-i < 4 {
				rem := data[i:]
				if !utf8.FullRune(rem) && partialUTF8(rem) {
					flushPlain()
					c.utfBuf = append(c.utfBuf[:0], rem...)
					return
				}
			}
			plain = append(plain, b)

		case stateESC:
			c.escBuf = append(c.escBuf, b)
			switch b {
			case '[':
				c.state = stateCSI
			case ']':
				c.state = stateOSC
			case 'P', 'X', '^', '_':
				c.state = stateString
			case '(', ')', '*', '+', '#':
				c.state = stateCharset
			default:
				// Single-byte ESC sequence (e.g. ESC =, ESC >, ESC c, ESC 7, ESC 8)
				if b == 'c' { // RIS full terminal reset
					c.altScreen, c.fgStyled, c.bgStyled = false, false, false
				}
				c.out = append(c.out, c.escBuf...)
				c.escBuf = c.escBuf[:0]
				c.state = stateNormal
			}

		case stateCharset:
			c.escBuf = append(c.escBuf, b)
			c.out = append(c.out, c.escBuf...)
			c.escBuf = c.escBuf[:0]
			c.state = stateNormal

		case stateCSI:
			c.escBuf = append(c.escBuf, b)
			if (b >= 0x40 && b <= 0x7e) || len(c.escBuf) >= maxEscBufSize {
				c.handleCSI(c.escBuf)
				c.out = append(c.out, c.escBuf...)
				c.escBuf = c.escBuf[:0]
				c.state = stateNormal
			}

		case stateOSC:
			c.escBuf = append(c.escBuf, b)
			switch {
			case b == 0x07 || len(c.escBuf) >= maxEscBufSize:
				c.out = append(c.out, c.escBuf...)
				c.escBuf = c.escBuf[:0]
				c.state = stateNormal
			case b == 0x1b:
				c.state = stateOSCEsc
			}

		case stateOSCEsc:
			c.escBuf = append(c.escBuf, b)
			if b == '\\' || len(c.escBuf) >= maxEscBufSize {
				c.out = append(c.out, c.escBuf...)
				c.escBuf = c.escBuf[:0]
				c.state = stateNormal
			} else if b != 0x1b {
				c.state = stateOSC
			}

		case stateString:
			c.escBuf = append(c.escBuf, b)
			if b == 0x1b {
				c.state = stateStringEsc
			} else if len(c.escBuf) >= maxEscBufSize {
				c.out = append(c.out, c.escBuf...)
				c.escBuf = c.escBuf[:0]
				c.state = stateNormal
			}

		case stateStringEsc:
			c.escBuf = append(c.escBuf, b)
			if b == '\\' || len(c.escBuf) >= maxEscBufSize {
				c.out = append(c.out, c.escBuf...)
				c.escBuf = c.escBuf[:0]
				c.state = stateNormal
			} else if b != 0x1b {
				c.state = stateString
			}
		}
	}
	flushPlain()
}

// partialUTF8 reports whether b looks like the valid prefix of a multi-byte
// UTF-8 rune that has not finished arriving yet.
func partialUTF8(b []byte) bool {
	if len(b) == 0 || len(b) >= 4 {
		return false
	}
	lead := b[0]
	var want int
	switch {
	case lead >= 0xc2 && lead <= 0xdf:
		want = 2
	case lead >= 0xe0 && lead <= 0xef:
		want = 3
	case lead >= 0xf0 && lead <= 0xf4:
		want = 4
	default:
		return false
	}
	if len(b) >= want {
		return false
	}
	for _, cont := range b[1:] {
		if cont&0xc0 != 0x80 {
			return false
		}
	}
	return true
}

// handleCSI updates altScreen and SGR color state from a complete CSI sequence.
func (c *conn) handleCSI(seq []byte) {
	if len(seq) < 3 || seq[0] != 0x1b || seq[1] != '[' {
		return
	}
	final := seq[len(seq)-1]
	params := string(seq[2 : len(seq)-1])
	switch final {
	case 'm':
		c.handleSGR(params)
	case 'h', 'l':
		if strings.HasPrefix(params, "?") {
			on := final == 'h'
			for _, part := range strings.Split(params[1:], ";") {
				if part == "1049" || part == "1047" || part == "47" {
					c.altScreen = on
				}
			}
		}
	}
}

// handleSGR tracks whether the remote application currently has an explicit
// foreground or background color active.
func (c *conn) handleSGR(params string) {
	if params == "" {
		c.fgStyled, c.bgStyled = false, false
		return
	}
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		if parts[i] == "" {
			c.fgStyled, c.bgStyled = false, false
			continue
		}
		// Colon-separated subparameters (e.g. 38:2::R:G:B)
		head := parts[i]
		if cut, _, ok := strings.Cut(head, ":"); ok {
			head = cut
		}
		code, err := strconv.Atoi(head)
		if err != nil {
			continue
		}
		switch {
		case code == 0:
			c.fgStyled, c.bgStyled = false, false
		case code == 39:
			c.fgStyled = false
		case code == 49:
			c.bgStyled = false
		case (code >= 30 && code <= 37) || (code >= 90 && code <= 97):
			c.fgStyled = true
		case (code >= 40 && code <= 47) || (code >= 100 && code <= 107):
			c.bgStyled = true
		case code == 38:
			c.fgStyled = true
			// Skip semicolon-encoded extended color arguments (38;5;N or 38;2;R;G;B)
			if !strings.Contains(parts[i], ":") && i+1 < len(parts) {
				if parts[i+1] == "5" {
					i += 2
				} else if parts[i+1] == "2" {
					i += 4
				}
			}
		case code == 48:
			c.bgStyled = true
			if !strings.Contains(parts[i], ":") && i+1 < len(parts) {
				if parts[i+1] == "5" {
					i += 2
				} else if parts[i+1] == "2" {
					i += 4
				}
			}
		}
	}
}

// Highlight applies MobaXterm-style keyword syntax highlighting to a single
// plain-text string (exposed for unit tests and previews).
func Highlight(s string) string {
	if len(s) < 2 {
		return s
	}
	return string(appendHighlighted(nil, []byte(s)))
}

// appendHighlighted scans plain (guaranteed free of ANSI escapes) and appends
// it to dst with MobaXterm-style ANSI SGR sequences around matched tokens.
func appendHighlighted(dst, plain []byte) []byte {
	// Skip single-character interactive echo immediately.
	if len(plain) < 2 {
		return append(dst, plain...)
	}
	matches := tokenRe.FindAllSubmatchIndex(plain, -1)
	if len(matches) == 0 {
		return append(dst, plain...)
	}
	pos := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		if start < pos {
			continue
		}
		var openSeq, closeSeq string
		switch {
		case m[2*idxURL] >= 0:
			// Trim trailing sentence punctuation (.,;:!) from URLs.
			for end > start && bytes.IndexByte([]byte(".,;:!"), plain[end-1]) >= 0 {
				end--
			}
			openSeq, closeSeq = sgrURL, sgrResetURL
		case m[2*idxIP] >= 0, m[2*idxMAC] >= 0:
			openSeq, closeSeq = sgrAddr, sgrResetFG
		case m[2*idxErr] >= 0:
			openSeq, closeSeq = sgrError, sgrResetFG
		case m[2*idxOK] >= 0:
			openSeq, closeSeq = sgrSuccess, sgrResetFG
		case m[2*idxWarn] >= 0:
			openSeq, closeSeq = sgrWarn, sgrResetFG
		case m[2*idxInfo] >= 0:
			openSeq, closeSeq = sgrInfo, sgrResetFG
		default:
			continue
		}
		dst = append(dst, plain[pos:start]...)
		dst = append(dst, openSeq...)
		dst = append(dst, plain[start:end]...)
		dst = append(dst, closeSeq...)
		pos = end
	}
	return append(dst, plain[pos:]...)
}
