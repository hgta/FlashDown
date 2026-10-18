package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hgta/flashdown/internal/engine"
)

func TestParseMinimal(t *testing.T) {
	opts, err := Parse([]string{"https://example.com/a.zip"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if opts.URL != "https://example.com/a.zip" {
		t.Errorf("URL = %q", opts.URL)
	}
	if opts.Quiet || opts.Jets != 0 {
		t.Errorf("unexpected defaults: %+v", opts)
	}
}

func TestParseAllFlags(t *testing.T) {
	opts, err := Parse([]string{
		"https://example.com/a.zip", "-o", `D:\DL`, "-n", "b.zip", "-j", "8", "-c", "games", "-q",
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if opts.Dir != `D:\DL` || opts.Name != "b.zip" || opts.Jets != 8 || opts.Category != "games" || !opts.Quiet {
		t.Errorf("opts = %+v", opts)
	}
}

// TestParseInterleaved covers the reason splitArgs exists: the stdlib flag
// package stops at the first positional, so `URL -j 8` only works if the flag
// and its value reach the FlagSet as a pair.
func TestParseInterleaved(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		url   string
		dir   string
		jets  int
		quiet bool
	}{
		{
			name: "flags before url",
			args: []string{"-o", `D:\DL`, "-j", "8", "https://example.com/a.zip"},
			url:  "https://example.com/a.zip",
			dir:  `D:\DL`,
			jets: 8,
		},
		{
			name:  "url between flags",
			args:  []string{"-o", `D:\DL`, "https://example.com/a.zip", "-j", "3", "-q"},
			url:   "https://example.com/a.zip",
			dir:   `D:\DL`,
			jets:  3,
			quiet: true,
		},
		{
			name: "equals form does not swallow the url",
			args: []string{"-j=6", "-o=D:\\DL", "https://example.com/a.zip"},
			url:  "https://example.com/a.zip",
			dir:  `D:\DL`,
			jets: 6,
		},
		{
			name:  "double dash ends parsing",
			args:  []string{"-q", "--", "https://example.com/a.zip"},
			url:   "https://example.com/a.zip",
			quiet: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts, err := Parse(c.args)
			if err != nil {
				t.Fatalf("Parse(%v): %v", c.args, err)
			}
			if opts.URL != c.url || opts.Dir != c.dir || opts.Jets != c.jets || opts.Quiet != c.quiet {
				t.Errorf("opts = %+v", opts)
			}
		})
	}
}

// A value is a value even when it looks like a flag: `-n -weird` names the file
// "-weird" rather than failing as an unknown flag.
func TestParseValueThatLooksLikeFlag(t *testing.T) {
	opts, err := Parse([]string{"-n", "-weird", "https://example.com/a.zip"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if opts.Name != "-weird" {
		t.Errorf("Name = %q", opts.Name)
	}
}

func TestParseRejectsUsage(t *testing.T) {
	cases := [][]string{
		{},
		{"ftp://example.com/a.zip"},
		{"not a url"},
		{"https://example.com/a.zip", "-j", "99"},
		{"https://example.com/a.zip", "-j", "-1"},
	}
	for _, args := range cases {
		if _, err := Parse(args); err == nil {
			t.Errorf("Parse(%v) accepted an invalid command line", args)
		}
	}
}

func TestParseVersionNeedsNoURL(t *testing.T) {
	opts, err := Parse([]string{"--version"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !opts.ShowVersion {
		t.Error("ShowVersion = false")
	}
}

func TestFormatProgress(t *testing.T) {
	line := FormatProgress(engine.TaskView{
		Size: 1000, Done: 500, Speed: 2048, RemainMs: 5000, Jets: 5,
	})

	for _, want := range []string{"50.0%", "500 B / 1000 B", "2.0 KB/s", "剩余 5s", "5 连接"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q missing %q", line, want)
		}
	}
}

func TestFormatProgressWithoutSize(t *testing.T) {
	line := FormatProgress(engine.TaskView{Done: 2048})
	if !strings.Contains(line, "0.0%") || strings.Contains(line, "剩余") {
		t.Errorf("unknown size should not invent a percentage or ETA: %q", line)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:         "0 B",
		1023:      "1023 B",
		1024:      "1.0 KB",
		1536:      "1.5 KB",
		1048576:   "1.0 MB",
		108506152: "103.5 MB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	cases := map[int64]string{
		400:     "<1s",
		5000:    "5s",
		65000:   "1m05s",
		3725000: "1h02m",
	}
	for in, want := range cases {
		if got := humanDuration(in); got != want {
			t.Errorf("humanDuration(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestIsTerminalForBuffer(t *testing.T) {
	if isTerminal(&bytes.Buffer{}) {
		t.Error("a buffer is not a terminal; progress must fall back to line mode")
	}
}

func TestUsageMentionsExitCodes(t *testing.T) {
	usage := Usage()
	for _, want := range []string{"flashdown-cli.exe", "-o", "130"} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage missing %q", want)
		}
	}
}
