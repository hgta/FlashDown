// Package cli is the headless entry point: same engine, no window.
//
// It exists because the GUI needs WebView2, and WebView2 stops at Windows 8.1.
// The download engine has no such dependency, so on an unsupported system this
// package still gets the bytes - which is the whole reason the project exists.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hgta/flashdown/internal/config"
	"github.com/hgta/flashdown/internal/engine"
)

// Exit codes are part of the contract: scripts branch on them.
const (
	ExitOK       = 0
	ExitFailed   = 1
	ExitUsage    = 2
	ExitInterrupt = 130
)

// Options is the parsed command line.
type Options struct {
	URL         string
	Dir         string
	Name        string
	Category    string
	Jets        int
	Quiet       bool
	ShowVersion bool
}

// Usage is printed for -h and for any usage error.
func Usage() string {
	return `用法：flashdown-cli.exe <链接> [选项]

选项：
  -o <目录>    保存目录（默认使用配置里的下载目录）
  -n <文件名>  指定文件名
  -j <数量>    连接数 1-30（默认取配置值）
  -c <类别>    类别 id
  -q           不输出进度
      --version  显示版本

退出码：0 完成 / 1 下载失败 / 2 用法错误 / 130 已中断（可续传）`
}

// Parse reads the command line. Help and version are not errors.
//
// Positional arguments are pulled out first so `URL -j 8` works as naturally as
// `-j 8 URL`; the stdlib flag package stops at the first positional.
func Parse(args []string) (Options, error) {
	flags, positional := splitArgs(args)

	fs := flag.NewFlagSet("flashdown-cli", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	var opts Options
	fs.StringVar(&opts.Dir, "o", "", "保存目录")
	fs.StringVar(&opts.Name, "n", "", "文件名")
	fs.IntVar(&opts.Jets, "j", 0, "连接数")
	fs.StringVar(&opts.Category, "c", "", "类别 id")
	fs.BoolVar(&opts.Quiet, "q", false, "不输出进度")
	fs.BoolVar(&opts.ShowVersion, "version", false, "显示版本")

	if err := fs.Parse(flags); err != nil {
		return Options{}, err
	}

	opts.URL = ""
	if len(positional) > 0 {
		opts.URL = strings.TrimSpace(positional[0])
	}
	if opts.ShowVersion {
		return opts, nil
	}
	if opts.URL == "" {
		return Options{}, errors.New("缺少下载链接")
	}

	u, err := url.Parse(opts.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Options{}, errors.New("链接必须是 http 或 https 地址")
	}

	if opts.Jets != 0 && (opts.Jets < 1 || opts.Jets > 30) {
		return Options{}, errors.New("连接数必须在 1 到 30 之间")
	}

	return opts, nil
}

// splitArgs separates flags from positional arguments, keeping a value-taking
// flag next to its value so the FlagSet sees the pair it expects.
func splitArgs(args []string) (flags, positional []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return flags, append(positional, args[i+1:]...)

		case strings.HasPrefix(arg, "-") && arg != "-":
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && flagTakesValue(arg) && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}

		default:
			positional = append(positional, arg)
		}
	}
	return flags, positional
}

func flagTakesValue(arg string) bool {
	name := strings.TrimLeft(arg, "-")
	switch name {
	case "o", "n", "j", "c":
		return true
	}
	return false
}

// Run downloads one file and reports through the exit code.
func Run(ctx context.Context, opts Options, stdout, stderr io.Writer) int {
	cfg, _, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "读取配置失败：%v\n", err)
		return ExitFailed
	}

	dir := opts.Dir
	if dir == "" {
		dir = cfg.DownloadRoot
	}
	if dir == "" {
		fmt.Fprintln(stderr, "未指定保存目录，且配置中没有下载目录")
		return ExitUsage
	}

	category := opts.Category
	if category == "" && len(cfg.Categories) > 0 {
		category = cfg.Categories[0].ID
	}
	if opts.Jets > 0 {
		// Process-local: changing the connection count for one command should not
		// rewrite the user's settings.
		cfg.MaxJets = opts.Jets
	}

	eng := engine.New(cfg)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		eng.Shutdown(shutdownCtx)
	}()

	// The `<name>.fd!` pair is the only record of an interrupted download, so it
	// has to be picked up before Add. Without this a second run of the same
	// command would silently start over from byte zero.
	if err := eng.RestoreDir(dir); err != nil {
		fmt.Fprintf(stderr, "读取未完成任务失败：%v\n", err)
	}

	// -n asks for a specific file, so a restored task under another name is a
	// different download, not a resume.
	task := eng.Find(opts.URL, dir)
	if task != nil && opts.Name != "" && task.Name() != opts.Name {
		task = nil
	}
	if task != nil {
		fmt.Fprintf(stdout, "继续未完成的下载：%s\n", task.Name())
	} else {
		task, err = eng.Add(engine.Spec{URL: opts.URL, Name: opts.Name, Dir: dir, Category: category})
		if err != nil {
			fmt.Fprintf(stderr, "无法创建任务：%v\n", err)
			return ExitUsage
		}
	}
	if err := eng.Start(task.ID); err != nil {
		fmt.Fprintf(stderr, "无法开始下载：%v\n", err)
		return ExitFailed
	}

	tty := isTerminal(stdout)
	quiet := opts.Quiet
	headerShown := false
	lastBucket := -1

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = eng.Pause(task.ID)
			waitFor(task, 5*time.Second, engine.StatePaused, engine.StateFinished)
			if !quiet {
				fmt.Fprintln(stdout)
			}
			if task.State() == engine.StateFinished {
				fmt.Fprintf(stdout, "已保存：%s\n", filepath.Join(dir, task.Name()))
				return ExitOK
			}
			fmt.Fprintln(stderr, "已中断，进度已保存；重新执行同一命令即可继续")
			return ExitInterrupt

		case <-ticker.C:
		}

		v := task.View()
		switch v.State {
		case engine.StateFinished:
			if !quiet {
				if tty {
					fmt.Fprintf(stdout, "\r\033[K")
				}
				fmt.Fprintln(stdout)
			}
			fmt.Fprintf(stdout, "已保存：%s\n", filepath.Join(dir, v.Name))
			return ExitOK

		case engine.StateError, engine.StateRemoved:
			if !quiet {
				fmt.Fprintln(stdout)
			}
			reason := v.Error
			if reason == "" {
				reason = string(v.State)
			}
			fmt.Fprintf(stderr, "下载失败：%s\n", reason)
			return ExitFailed
		}

		if quiet {
			continue
		}

		if !headerShown && v.Name != "" {
			jets := v.Jets
			if jets < 1 {
				jets = 1
			}
			fmt.Fprintf(stdout, "%s  %s  %d 连接\n", v.Name, humanBytes(v.Size), jets)
			headerShown = true
		}
		if !headerShown {
			continue
		}

		line := FormatProgress(v)
		if tty {
			// \033[K clears the tail of a shorter line, so numbers never smear.
			fmt.Fprintf(stdout, "\r%s\033[K", line)
			continue
		}

		// A redirected stream must not receive four lines per second.
		bucket := int(percentOf(v) / 10 * 10)
		if bucket > lastBucket {
			lastBucket = bucket
			fmt.Fprintln(stdout, line)
		}
	}
}

// FormatProgress renders one status line for a task snapshot.
func FormatProgress(v engine.TaskView) string {
	parts := []string{
		fmt.Sprintf("%5.1f%%", percentOf(v)),
		fmt.Sprintf("%s / %s", humanBytes(v.Done), humanBytes(v.Size)),
	}
	if v.Speed > 0 {
		parts = append(parts, humanBytes(v.Speed)+"/s")
	}
	if v.RemainMs > 0 {
		parts = append(parts, "剩余 "+humanDuration(v.RemainMs))
	}
	if v.Jets > 1 {
		parts = append(parts, fmt.Sprintf("%d 连接", v.Jets))
	}
	return strings.Join(parts, "  ")
}

func percentOf(v engine.TaskView) float64 {
	if v.Size <= 0 {
		return 0
	}
	return float64(v.Done) * 100 / float64(v.Size)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func humanDuration(ms int64) string {
	if ms < 1000 {
		return "<1s"
	}

	seconds := ms / 1000
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm%02ds", minutes, seconds%60)
	}
	return fmt.Sprintf("%dh%02dm", minutes/60, minutes%60)
}

// isTerminal distinguishes an interactive console from a redirected stream.
// A plain io.Writer (tests) is never a terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// waitFor polls until the task reaches one of the wanted states. Pause persists
// asynchronously, so this cannot be assumed to be instant.
func waitFor(task *engine.Task, limit time.Duration, want ...engine.State) bool {
	deadline := time.Now().Add(limit)
	for {
		state := task.State()
		for _, w := range want {
			if state == w {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}
