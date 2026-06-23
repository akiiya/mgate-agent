package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"mgate-agent/internal/app"
	"mgate-agent/internal/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, app.ErrUsage) {
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return app.ErrUsage
	}

	switch args[0] {
	case "version":
		fmt.Printf("%s %s\n", app.Name, app.Version)
		return nil
	case "config":
		if len(args) == 2 && args[1] == "default" {
			fmt.Print(config.DefaultConfigYAML)
			return nil
		}
		usage()
		return app.ErrUsage
	case "check":
		fs := flag.NewFlagSet("check", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		configPath := fs.String("config", config.DefaultPath, "config file path")
		if err := fs.Parse(args[1:]); err != nil {
			return app.ErrUsage
		}
		return app.Check(context.Background(), app.Options{ConfigPath: *configPath, Stdout: os.Stdout, Stderr: os.Stderr})
	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		configPath := fs.String("config", config.DefaultPath, "config file path")
		if err := fs.Parse(args[1:]); err != nil {
			return app.ErrUsage
		}
		return app.Doctor(context.Background(), app.Options{ConfigPath: *configPath, Stdout: os.Stdout, Stderr: os.Stderr})
	case "run":
		fs := flag.NewFlagSet("run", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		configPath := fs.String("config", config.DefaultPath, "config file path")
		if err := fs.Parse(args[1:]); err != nil {
			return app.ErrUsage
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return app.Run(ctx, app.Options{ConfigPath: *configPath, Stdout: os.Stdout, Stderr: os.Stderr})
	default:
		usage()
		return app.ErrUsage
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  %s run --config %s\n", app.Name, config.DefaultPath)
	fmt.Fprintf(os.Stderr, "  %s check --config %s\n", app.Name, config.DefaultPath)
	fmt.Fprintf(os.Stderr, "  %s doctor --config %s\n", app.Name, config.DefaultPath)
	fmt.Fprintf(os.Stderr, "  %s version\n", app.Name)
	fmt.Fprintf(os.Stderr, "  %s config default\n", app.Name)
}
