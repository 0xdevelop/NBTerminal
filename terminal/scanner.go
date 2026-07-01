package terminal

import (
	"bufio"
	"io"
)

const maxTerminalLineBytes = 10 * 1024 * 1024

// newLineScanner returns a Scanner sized for terminal output/history records.
// bufio.Scanner's default 64 KiB token limit is too small for real command
// output (for example minified JSON, certificates, or a long single log line),
// so the terminal core uses this helper consistently for stdout/stderr and JSONL
// history reads.
func newLineScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), maxTerminalLineBytes)
	return s
}
