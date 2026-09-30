package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	gos7logo "github.com/axon-expert/gos7-logo-client/logo"
)

const usage = `Usage:
  logo [options] read ADDRESS [ADDRESS...]
  logo [options] write ADDRESS VALUE [ADDRESS VALUE...]
  logo [options] watch [-interval DURATION] ADDRESS [ADDRESS...]

Addresses:
  V3       byte at offset 3
  V3.2     bit 2 at offset 3
  VW4      word at offset 4
  VD8      double word at offset 8
  V3-V5    inclusive range (read and watch only)

Connection options:
`

type logoClient interface {
	Connect(context.Context) error
	Read(gos7logo.VmAddr) (uint32, error)
	Stream(context.Context, time.Duration, ...gos7logo.VmAddr) (<-chan gos7logo.StreamResult, error)
	Write(gos7logo.VmAddr, uint32) error
	Disconnect() error
}

type clientFactory func(gos7logo.Config) logoClient

type commandConfig struct {
	host      string
	port      uint
	localTSAP gos7logo.TSAP
	logoTSAP  gos7logo.TSAP
	format    string
}

func main() {
	os.Exit(realMain())
}

func realMain() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go cancelOnSignal(ctx, signals, os.Stdout, cancel)

	err := run(ctx, os.Args[1:], os.Stdout, os.Stderr,
		func(config gos7logo.Config) logoClient {
			return gos7logo.NewClient(config)
		})
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}
	fmt.Fprintf(os.Stderr, "logo: %v\n", err)
	return 1
}

func cancelOnSignal(
	ctx context.Context,
	signals <-chan os.Signal,
	output io.Writer,
	cancel context.CancelFunc,
) {
	select {
	case <-signals:
		_, _ = io.WriteString(output, "\r")
		cancel()
	case <-ctx.Done():
	}
}

func run(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	newClient clientFactory,
) (err error) {
	options := flag.NewFlagSet("logo", flag.ContinueOnError)
	options.SetOutput(stderr)
	defaults := gos7logo.NewConfig("localhost")
	cfg := commandConfig{
		host:      defaults.Host,
		port:      uint(defaults.Port),
		localTSAP: defaults.LocalTSAP,
		logoTSAP:  defaults.RemoteTSAP,
		format:    "d",
	}
	options.StringVar(
		&cfg.host,
		"host",
		cfg.host,
		"controller hostname or IP address",
	)
	options.UintVar(&cfg.port, "port", cfg.port, "controller TCP port")
	options.TextVar(
		&cfg.localTSAP,
		"local-tsap",
		cfg.localTSAP,
		"local TSAP (XX.XX, decimal, or 0x-prefixed)",
	)
	options.TextVar(
		&cfg.logoTSAP,
		"logo-tsap",
		cfg.logoTSAP,
		"LOGO TSAP (XX.XX, decimal, or 0x-prefixed)",
	)
	options.StringVar(
		&cfg.format,
		"f",
		cfg.format,
		"output format: d, x/h, b, or o; append _ to group digits",
	)
	options.Usage = func() {
		_, _ = fmt.Fprint(stderr, usage)
		options.PrintDefaults()
	}
	if err := options.Parse(args); err != nil {
		return err
	}
	if options.NArg() == 0 {
		options.Usage()
		return errors.New("command is required")
	}
	if cfg.port > 0xffff {
		return errors.New("port must be between 0 and 65535")
	}
	if err := validateOutputFormat(cfg.format); err != nil {
		return err
	}
	command := options.Arg(0)
	commandArgs := options.Args()[1:]
	var addresses []gos7logo.VmAddr
	var values []uint32
	interval := time.Second

	switch command {
	case "read":
		addresses, err = parseAddresses(commandArgs)
	case "write":
		addresses, values, err = parseWrites(commandArgs)
	case "watch":
		watchFlags := flag.NewFlagSet("logo watch", flag.ContinueOnError)
		watchFlags.SetOutput(stderr)
		watchFlags.DurationVar(&interval, "interval", time.Second, "polling interval")
		if err = watchFlags.Parse(commandArgs); err == nil {
			if interval <= 0 {
				err = errors.New("watch interval must be greater than zero")
			} else {
				addresses, err = parseAddresses(watchFlags.Args())
			}
		}
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		return err
	}

	clientConfig := gos7logo.NewConfig(cfg.host)
	clientConfig.Port = uint16(cfg.port)
	clientConfig.LocalTSAP = cfg.localTSAP
	clientConfig.RemoteTSAP = cfg.logoTSAP
	client := newClient(clientConfig)
	if err := client.Connect(ctx); err != nil {
		endpoint := net.JoinHostPort(cfg.host, strconv.FormatUint(uint64(cfg.port), 10))
		return fmt.Errorf("connect to %s: %w", endpoint, err)
	}
	defer func() {
		if disconnectErr := client.Disconnect(); err == nil && disconnectErr != nil {
			err = fmt.Errorf("disconnect: %w", disconnectErr)
		}
	}()

	switch command {
	case "read":
		return readValues(client, addresses, cfg.format, stdout)
	case "write":
		return writeValues(client, addresses, values, cfg.format, stdout)
	case "watch":
		return watchValues(ctx, client, addresses, interval, cfg.format, stdout)
	default:
		panic("unreachable")
	}
}

func parseAddresses(args []string) ([]gos7logo.VmAddr, error) {
	if len(args) == 0 {
		return nil, errors.New("at least one address is required")
	}
	addresses := make([]gos7logo.VmAddr, 0, len(args))
	for _, raw := range args {
		expanded, err := parseAddressOrRange(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid address %q: %w", raw, err)
		}
		addresses = append(addresses, expanded...)
	}
	return addresses, nil
}

func parseAddressOrRange(raw string) ([]gos7logo.VmAddr, error) {
	startRaw, endRaw, isRange := strings.Cut(raw, "-")
	start, err := gos7logo.NewVmAddrFromString(startRaw)
	if err != nil {
		return nil, err
	}
	if !isRange {
		return []gos7logo.VmAddr{start}, nil
	}
	end, err := gos7logo.NewVmAddrFromString(endRaw)
	if err != nil {
		return nil, err
	}
	if start.Type != end.Type {
		return nil, fmt.Errorf("range endpoints must have the same type: %s and %s", start, end)
	}
	return expandAddressRange(start, end)
}

func expandAddressRange(start, end gos7logo.VmAddr) ([]gos7logo.VmAddr, error) {
	const maxRangeLength = 1 << 20

	startIndex := uint64(start.Byte)
	endIndex := uint64(end.Byte)
	if start.Type == gos7logo.Bit {
		startIndex = startIndex*8 + uint64(start.Bit)
		endIndex = endIndex*8 + uint64(end.Bit)
	}
	if endIndex < startIndex {
		return nil, fmt.Errorf("range end %s precedes start %s", end, start)
	}
	length := endIndex - startIndex + 1
	if length > maxRangeLength {
		return nil, fmt.Errorf("range contains %d addresses; maximum is %d", length, maxRangeLength)
	}

	addresses := make([]gos7logo.VmAddr, int(length))
	for i := range addresses {
		index := startIndex + uint64(i)
		byteAddr := uint32(index)
		var bit uint8
		if start.Type == gos7logo.Bit {
			byteAddr = uint32(index / 8)
			bit = uint8(index % 8)
		}
		addresses[i] = gos7logo.MustNewVmAddr(start.Type, byteAddr, bit)
	}
	return addresses, nil
}

func parseWrites(args []string) ([]gos7logo.VmAddr, []uint32, error) {
	if len(args) == 0 || len(args)%2 != 0 {
		return nil, nil, errors.New("write requires ADDRESS VALUE pairs")
	}
	addresses := make([]gos7logo.VmAddr, 0, len(args)/2)
	values := make([]uint32, 0, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		addr, err := gos7logo.NewVmAddrFromString(args[i])
		if err != nil {
			return nil, nil, fmt.Errorf("invalid address %q: %w", args[i], err)
		}
		value, err := strconv.ParseUint(args[i+1], 0, 32)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid value %q: %w", args[i+1], err)
		}
		if err := validateValue(addr, uint32(value)); err != nil {
			return nil, nil, fmt.Errorf("invalid value for %s: %w", args[i], err)
		}
		addresses = append(addresses, addr)
		values = append(values, uint32(value))
	}
	return addresses, values, nil
}

func validateValue(addr gos7logo.VmAddr, value uint32) error {
	switch addr.Type {
	case gos7logo.Bit:
		if value > 1 {
			return errors.New("bit value must be 0 or 1")
		}
	case gos7logo.Byte:
		if value > 0xff {
			return errors.New("byte value must be between 0 and 255")
		}
	case gos7logo.Word:
		if value > 0xffff {
			return errors.New("word value must be between 0 and 65535")
		}
	}
	return nil
}

func validateOutputFormat(format string) error {
	baseFormat := strings.TrimSuffix(format, "_")
	if strings.Count(format, "_") > 1 || strings.Contains(baseFormat, "_") {
		return fmt.Errorf(
			"invalid output format %q: expected d, x, h, b, or o, optionally followed by _",
			format,
		)
	}
	switch baseFormat {
	case "d", "x", "h", "b", "o":
		return nil
	default:
		return fmt.Errorf(
			"invalid output format %q: expected d, x, h, b, or o, optionally followed by _",
			format,
		)
	}
}

func formatValue(addr gos7logo.VmAddr, value uint32, format string) string {
	grouped := strings.HasSuffix(format, "_")
	format = strings.TrimSuffix(format, "_")
	base := 10
	width := 0
	groupSize := 3
	bits := addr.Type.Size() * 8
	if addr.Type == gos7logo.Bit {
		bits = 1
	}
	switch format {
	case "x", "h":
		base = 16
		width = (bits + 3) / 4
		groupSize = 2
	case "b":
		base = 2
		width = bits
		groupSize = 4
	case "o":
		base = 8
		width = (bits + 2) / 3
	}
	formatted := strconv.FormatUint(uint64(value), base)
	formatted = strings.Repeat("0", max(0, width-len(formatted))) + formatted
	if grouped {
		return groupDigits(formatted, groupSize)
	}
	return formatted
}

func groupDigits(value string, size int) string {
	firstGroupSize := len(value) % size
	if firstGroupSize == 0 {
		firstGroupSize = size
	}
	var grouped strings.Builder
	grouped.Grow(len(value) + len(value)/size)
	grouped.WriteString(value[:firstGroupSize])
	for i := firstGroupSize; i < len(value); i += size {
		grouped.WriteByte('_')
		grouped.WriteString(value[i : i+size])
	}
	return grouped.String()
}

func readValues(
	client logoClient,
	addresses []gos7logo.VmAddr,
	format string,
	output io.Writer,
) error {
	for _, addr := range addresses {
		value, err := client.Read(addr)
		if err != nil {
			return fmt.Errorf("read %s: %w", addr, err)
		}
		if _, err := fmt.Fprintf(
			output,
			"%s=%s\n",
			addr,
			formatValue(addr, value, format),
		); err != nil {
			return fmt.Errorf("print value: %w", err)
		}
	}
	return nil
}

func writeValues(
	client logoClient,
	addresses []gos7logo.VmAddr,
	values []uint32,
	format string,
	output io.Writer,
) error {
	for i, addr := range addresses {
		if err := client.Write(addr, values[i]); err != nil {
			return fmt.Errorf("write %s: %w", addr, err)
		}
		if _, err := fmt.Fprintf(
			output,
			"%s=%s\n",
			addr,
			formatValue(addr, values[i], format),
		); err != nil {
			return fmt.Errorf("print value: %w", err)
		}
	}
	return nil
}

func watchValues(
	ctx context.Context,
	client logoClient,
	addresses []gos7logo.VmAddr,
	interval time.Duration,
	format string,
	output io.Writer,
) error {
	stream, err := client.Stream(ctx, interval, addresses...)
	if err != nil {
		return err
	}
	for result := range stream {
		if result.Err != nil {
			return result.Err
		}
		if err := printSample(result.Data, format, output); err != nil {
			return err
		}
	}
	return nil
}

func printSample(
	values gos7logo.VmAddrValues,
	format string,
	output io.Writer,
) error {
	var sample bytes.Buffer
	_, _ = fmt.Fprint(&sample, time.Now().Format(time.RFC3339))
	for _, value := range values {
		_, _ = fmt.Fprintf(
			&sample,
			" %s=%s",
			value.VmAddr,
			formatValue(value.VmAddr, value.Value, format),
		)
	}
	if err := sample.WriteByte('\n'); err != nil {
		return fmt.Errorf("build sample: %w", err)
	}
	if _, err := sample.WriteTo(output); err != nil {
		return fmt.Errorf("print sample: %w", err)
	}
	return nil
}
