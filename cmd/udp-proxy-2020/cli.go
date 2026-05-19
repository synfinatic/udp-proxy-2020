package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
)

type CLI struct {
	Interface      []string `kong:"short='i',help='Two or more interfaces to use'"`
	FixedIp        []string `kong:"short='I',help='IPs to always send to iface@ip'"`
	Port           []int32  `kong:"short='p',help='One or more UDP ports to process'"`
	Timeout        int64    `kong:"short='t',default=250,help='Timeout in msec'"`
	CacheTTL       int64    `kong:"short='T',default=180,help='Client IP cache TTL in minutes'"`
	DeliverLocal   bool     `kong:"short='l',help='Deliver packets locally over loopback'"`
	Level          string   `kong:"short='L',default='info',enum='trace,debug,info,warn,error',help='Log level [trace|debug|info|warn|error]'"`
	LogLines       bool     `kong:"help='Print line number in logs'"`
	Logfile        string   `kong:"default='stderr',help='Write logs to filename'"`
	NoListen       bool     `kong:"help='Do not listen locally on UDP ports'"`
	Decode         bool     `kong:"help='Print packet decodes to stdout similar to tcpdump -e'"`
	Pcap           bool     `kong:"short='P',help='Generate pcap files for debugging'"`
	PcapPath       string   `kong:"short='d',default='/root',help='Directory to write debug pcap files'"`
	GraphPipeline  string   `kong:"help='Generate Graphviz dot file for pipelines at specified path'"`
	ListInterfaces bool     `kong:"help='List available interfaces and exit'"`
	Version        bool     `kong:"short='v',help='Print version information'"`
}

func parseArgs() CLI {
	cli := CLI{}
	parser := kong.Must(
		&cli,
		kong.Name("udp-proxy-2020"),
		kong.Description("A crappy UDP proxy for the year 2020 and beyond!"),
		kong.UsageOnError(),
	)
	_, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)

	if cli.Version {
		delta := ""
		if len(Delta) > 0 {
			delta = fmt.Sprintf(" [%s delta]", Delta)
			Tag = "Unknown"
		}
		fmt.Printf("udp-proxy-2020 Version %s -- Copyright 2020-2026 Aaron Turner\n", Version)
		fmt.Printf("%s (%s)%s built at %s\n", CommitID, Tag, delta, Buildinfos)
		os.Exit(0)
	}

	if len(cli.Interface) < 2 && !cli.ListInterfaces && !cli.Version && cli.GraphPipeline == "" {
		fmt.Fprintf(os.Stderr, "Please specify two or more --interface\n")
		os.Exit(1)
	}
	if len(cli.Port) < 1 && !cli.ListInterfaces && !cli.Version && cli.GraphPipeline == "" {
		fmt.Fprintf(os.Stderr, "Please specify one or more --port\n")
		os.Exit(1)
	}

	if cli.GraphPipeline != "" {
		if len(cli.Interface) < 2 {
			fmt.Fprintf(os.Stderr, "Please specify two or more --interface for --graph-pipeline\n")
			os.Exit(1)
		}
		if len(cli.Port) < 1 {
			fmt.Fprintf(os.Stderr, "Please specify one or more --port for --graph-pipeline\n")
			os.Exit(1)
		}
	}

	return cli
}

func setupLogging(cli CLI) {
	var level slog.Level
	switch cli.Level {
	case "trace":
		level = slog.LevelDebug - 4
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "info":
		level = slog.LevelInfo
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cli.LogLines,
	}

	var handler slog.Handler
	if cli.Logfile != "stderr" {
		file, err := os.OpenFile(cli.Logfile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to open log file %s: %v\n", cli.Logfile, err)
			os.Exit(1)
		}
		handler = slog.NewTextHandler(file, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))
}
