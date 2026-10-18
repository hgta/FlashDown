// flashdown-cli is the windowless entry point.
//
// It must never import app or Wails: Go runs package init before main, and the
// GUI packages carry the very dependency this binary exists to avoid.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/hgta/flashdown/internal/cli"
	"github.com/hgta/flashdown/internal/startup"
	"github.com/hgta/flashdown/internal/version"
)

func main() {
	opts, err := cli.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stdout, cli.Usage())
			os.Exit(cli.ExitOK)
		}
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, cli.Usage())
		os.Exit(cli.ExitUsage)
	}

	if opts.ShowVersion {
		fmt.Fprintf(os.Stdout, "flashdown-cli %s\n", version.Value)
		return
	}

	info, err := startup.CheckFor(startup.ModeCLI, nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "启动检查失败：%v\n", err)
		os.Exit(cli.ExitFailed)
	}
	if info.Unsupported {
		startup.WarnPlatform(info)
		fmt.Fprintf(os.Stderr, "警告：当前系统 Windows %s 不受支持，将以兼容模式继续\n", info.VersionText())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.Exit(cli.Run(ctx, opts, os.Stdout, os.Stderr))
}
