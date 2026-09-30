package gos7logo

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	gos7patch "github.com/axon-expert/gos7-logo-client/gos7-patch"
)

type Client interface {
	Connect(ctx context.Context) error
	Read(addr VMAddr) (uint32, error)
	ReadMany(addrs ...VMAddr) (VMAddrValues, error)
	ReadManyTo(buf []byte, addrs ...VMAddr) error
	Stream(
		ctx context.Context, interval time.Duration, addrs ...VMAddr,
	) (<-chan StreamResult, error)
	Write(addr VMAddr, value uint32) error
	WriteMany(addrs ...VMAddrValue) error
	Disconnect() error
}

var ErrNotConnected = errors.New("client is not connected")
var ErrReadOnlyAddress = errors.New("address is read-only")

type StreamResult struct {
	Data VMAddrValues
	Err  error
}

var _ Client = &client{}

type client struct {
	helper    gos7patch.Helper
	client    gos7patch.Client
	handler   *gos7patch.TCPClientHandler
	area      string
	dbNumber  int
	connected atomic.Bool
}

func NewClient(config Config) *client {
	handler := gos7patch.NewTCPClientHandlerWithTSAP(
		config.endpoint(),
		uint16(config.LocalTSAP),
		uint16(config.RemoteTSAP),
	)
	return &client{
		area: "DB", dbNumber: 1,
		client:  gos7patch.NewClient(handler),
		handler: handler}
}

func (c *client) Connect(ctx context.Context) error {
	c.connected.Store(false)
	if err := c.handler.ConnectContext(ctx); err != nil {
		_ = c.handler.Close()
		return fmt.Errorf("connect: %w", err)
	}
	c.connected.Store(true)
	return nil
}

func (c *client) ensureConnected() error {
	if !c.connected.Load() {
		return ErrNotConnected
	}
	return nil
}

func (c *client) Write(addr VMAddr, value uint32) error {
	if err := c.ensureConnected(); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := addr.Validate(); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if addr.Type == Output {
		return fmt.Errorf("write %s: %w", addr, ErrReadOnlyAddress)
	}
	size := addr.Type.Size()
	buff := make([]byte, size)
	if addr.Type == Bit {
		if err := c.client.AGReadDB(c.dbNumber, int(addr.Byte), size, buff); err != nil {
			return err
		}
	}
	if err := c.writeToBuffer(addr, buff, value); err != nil {
		return err
	}
	if err := c.client.AGWriteDB(c.dbNumber, int(addr.Byte), size, buff); err != nil {
		return err
	}
	return nil
}

func (c *client) WriteMany(args ...VMAddrValue) error {
	if len(args) == 0 {
		return fmt.Errorf("failed `WriteMany`: args is empty")
	}
	if err := c.ensureConnected(); err != nil {
		return fmt.Errorf("WriteMany: %w", err)
	}
	addrs := make([]VMAddr, len(args))
	for i, arg := range args {
		if arg.VMAddr.Type == Output {
			return fmt.Errorf("WriteMany: %s: %w", arg.VMAddr, ErrReadOnlyAddress)
		}
		addrs[i] = arg.VMAddr
	}
	start, size, err := vmAddrRange(addrs)
	if err != nil {
		return fmt.Errorf("WriteMany: %w", err)
	}
	buff := make([]byte, size)
	if err := c.client.AGReadDB(c.dbNumber, int(start), size, buff); err != nil {
		return err
	}
	for _, val := range args {
		offset := int(val.VMAddr.Byte - start)
		if err := c.writeToBuffer(val.VMAddr, buff[offset:], val.Value); err != nil {
			return err
		}
	}
	if err := c.client.AGWriteDB(c.dbNumber, int(start), size, buff); err != nil {
		return err
	}
	return nil
}

func (c *client) writeToBuffer(addr VMAddr, buff []byte, value uint32) error {
	if err := addr.Validate(); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if len(buff) < addr.Type.Size() {
		return fmt.Errorf("write: buffer too small for type %v", addr.Type)
	}
	switch addr.Type {
	case Bit, Output:
		if value > 0 {
			buff[0] |= 1 << addr.Bit
		} else {
			buff[0] &^= 1 << addr.Bit
		}
	case Byte:
		c.helper.SetValueAt(buff, 0, uint8(value))
	case DWord:
		c.helper.SetValueAt(buff, 0, uint32(value))
	case Real:
		c.helper.SetValueAt(buff, 0, float32(value))
	case Word, Counter, Timer:
		c.helper.SetValueAt(buff, 0, uint16(value))
	default:
		return errors.New("write: unknown data type")
	}

	return nil
}

func (c *client) Read(addr VMAddr) (uint32, error) {
	if err := c.ensureConnected(); err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	if err := addr.Validate(); err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	size := addr.Type.Size()
	buff := make([]byte, size)
	if err := c.client.AGReadDB(c.dbNumber, int(addr.Byte), size, buff); err != nil {
		return 0, err
	}
	result, err := c.getIntFromBuffer(addr, buff)
	if err != nil {
		return 0, err
	}
	return result, nil
}

func (c *client) ReadMany(args ...VMAddr) (VMAddrValues, error) {
	if len(args) == 0 {
		return nil, nil
	}
	start, size, err := vmAddrRange(args)
	if err != nil {
		return nil, fmt.Errorf("ReadMany: %w", err)
	}
	buff := make([]byte, size)
	if err := c.ReadManyTo(buff, args...); err != nil {
		return nil, err
	}

	values := make(VMAddrValues, len(args))
	for i, addr := range args {
		offset := int(addr.Byte - start)
		value, err := c.getIntFromBuffer(addr, buff[offset:])
		if err != nil {
			return nil, fmt.Errorf("ReadMany: decode %s: %w", addr, err)
		}
		values[i] = VMAddrValue{VMAddr: addr, Value: value}
	}
	return values, nil
}

func (c *client) ReadManyTo(buff []byte, args ...VMAddr) error {
	if len(args) == 0 {
		return nil
	}
	if err := c.ensureConnected(); err != nil {
		return fmt.Errorf("ReadManyTo: %w", err)
	}
	start, size, err := vmAddrRange(args)
	if err != nil {
		return fmt.Errorf("ReadManyTo: %w", err)
	}
	if len(buff) < size {
		return fmt.Errorf("ReadManyTo: need %d bytes, but buffer only %d bytes", size, len(buff))
	}
	if err := c.client.AGReadDB(c.dbNumber, int(start), size, buff); err != nil {
		return err
	}
	return nil
}

func (c *client) Stream(
	ctx context.Context, interval time.Duration, addrs ...VMAddr,
) (<-chan StreamResult, error) {
	if interval <= 0 {
		return nil, errors.New("stream interval must be greater than zero")
	}
	if len(addrs) == 0 {
		return nil, errors.New("stream addresses are empty")
	}
	if err := c.ensureConnected(); err != nil {
		return nil, fmt.Errorf("stream: %w", err)
	}
	if _, _, err := vmAddrRange(addrs); err != nil {
		return nil, fmt.Errorf("stream: %w", err)
	}

	streamAddrs := append([]VMAddr(nil), addrs...)
	results := make(chan StreamResult, 1)
	go c.stream(ctx, interval, streamAddrs, results)
	return results, nil
}

func (c *client) stream(
	ctx context.Context,
	interval time.Duration,
	addrs []VMAddr,
	results chan<- StreamResult,
) {
	defer close(results)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		data, err := c.ReadMany(addrs...)
		select {
		case results <- StreamResult{Data: data, Err: err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func (c *client) getIntFromBuffer(addr VMAddr, buff []byte) (uint32, error) {
	if err := addr.Validate(); err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	if len(buff) < addr.Type.Size() {
		return 0, fmt.Errorf("buffer too small for type %v", addr.Type)
	}
	switch addr.Type {
	case Bit, Output:
		var result uint8
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result >> addr.Bit & 1), nil
	case Byte:
		var result uint8
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result), nil
	case Word, Counter, Timer:
		var result uint16
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result), nil
	case DWord:
		var result uint32
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result), nil
	case Real:
		var result float32
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result), nil
	}

	return 0, errors.New("read: unknown data type")
}

func vmAddrRange(addrs []VMAddr) (uint32, int, error) {
	if len(addrs) == 0 {
		return 0, 0, errors.New("addresses are empty")
	}
	start := addrs[0].Byte
	var end uint64
	for _, addr := range addrs {
		if err := addr.Validate(); err != nil {
			return 0, 0, err
		}
		if addr.Byte < start {
			start = addr.Byte
		}
		addrEnd := uint64(addr.Byte) + uint64(addr.Type.Size())
		if addrEnd > end {
			end = addrEnd
		}
	}
	span := end - uint64(start)
	if span > uint64(^uint(0)>>1) {
		return 0, 0, errors.New("address range is too large")
	}
	return start, int(span), nil
}

func (c *client) Disconnect() error {
	c.connected.Store(false)
	return c.handler.Close()
}
