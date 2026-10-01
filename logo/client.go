package gos7logo

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	gos7patch "github.com/axon-expert/gos7-logo-client/gos7-patch"
)

type Client interface {
	Read(ctx context.Context, addr VMAddr) (uint32, error)
	ReadMany(ctx context.Context, addrs ...VMAddr) (VMAddrValues, error)
	ReadManyTo(ctx context.Context, buf []byte, addrs ...VMAddr) error
	Stream(
		ctx context.Context, interval time.Duration, addrs ...VMAddr,
	) (<-chan StreamResult, error)
	Write(ctx context.Context, addr VMAddr, value uint32) error
	WriteMany(ctx context.Context, addrs ...VMAddrValue) error
	Close() error
}

var ErrNotConnected = errors.New("client is not connected")
var ErrReadOnlyAddress = errors.New("address is read-only")
var ErrClientClosed = errors.New("client is closed")

type StreamResult struct {
	Data VMAddrValues
	Err  error
}

type connectionState uint8

const (
	connectionInitial connectionState = iota
	connectionConnected
	connectionLost
	connectionClosed
)

var _ Client = &client{}

type client struct {
	helper                gos7patch.Helper
	client                gos7patch.Client
	handler               *gos7patch.TCPClientHandler
	area                  string
	dbNumber              int
	reconnect             bool
	operationGate         chan struct{}
	lifecycleCtx          context.Context //nolint:containedctx // owns the client's lifecycle
	lifecycleCancel       context.CancelFunc
	closeMu               sync.Mutex
	connectionState       connectionState
	initialReconnectDelay time.Duration
	maxReconnectDelay     time.Duration
	reconnectDelay        time.Duration
	reconnectAt           time.Time
}

func NewClient(config Config) *client {
	handler := gos7patch.NewTCPClientHandlerWithTSAP(
		config.endpoint(),
		uint16(config.LocalTSAP),
		uint16(config.RemoteTSAP),
	)
	initialReconnectDelay, maxReconnectDelay := config.reconnectDelays()
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	operationGate := make(chan struct{}, 1)
	operationGate <- struct{}{}
	return &client{
		area:                  "DB",
		dbNumber:              1,
		reconnect:             config.Reconnect,
		initialReconnectDelay: initialReconnectDelay,
		maxReconnectDelay:     maxReconnectDelay,
		client:                gos7patch.NewClient(handler),
		handler:               handler,
		operationGate:         operationGate,
		lifecycleCtx:          lifecycleCtx,
		lifecycleCancel:       lifecycleCancel,
	}
}

func (c *client) connect(ctx context.Context) error {
	if c.connectionState == connectionClosed {
		return ErrClientClosed
	}
	if c.connectionState == connectionConnected {
		return nil
	}
	if c.connectionState == connectionLost && !c.reconnect {
		return ErrNotConnected
	}
	if delay := time.Until(c.reconnectAt); !c.reconnectAt.IsZero() && delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := c.handler.ConnectContext(ctx); err != nil {
		if ctx.Err() != nil {
			_ = c.handler.Close()
			return fmt.Errorf("connect: %w", ctx.Err())
		}
		delay := c.connectionFailed()
		return fmt.Errorf("connect: %w", reconnectError(err, delay))
	}
	c.connectionState = connectionConnected
	c.reconnectDelay = 0
	c.reconnectAt = time.Time{}
	return nil
}

func (c *client) ensureConnected(ctx context.Context) error {
	if c.connectionState == connectionConnected {
		return nil
	}
	reconnecting := c.connectionState == connectionLost && c.reconnect
	err := c.connect(ctx)
	if err != nil && reconnecting {
		return fmt.Errorf("reconnect: %w", err)
	}
	return err
}

func (c *client) connectionFailed() time.Duration {
	c.connectionState = connectionLost
	_ = c.handler.Close()
	if !c.reconnect {
		return 0
	}
	if c.reconnectDelay == 0 {
		c.reconnectDelay = c.initialReconnectDelay
	}
	delay := c.reconnectDelay
	c.reconnectAt = time.Now().Add(delay)
	c.reconnectDelay = min(c.reconnectDelay*2, c.maxReconnectDelay)
	return delay
}

func (c *client) operationFailed(err error) error {
	if IsPLCError(err) {
		return err
	}
	return reconnectError(err, c.connectionFailed())
}

func reconnectError(err error, delay time.Duration) error {
	if delay <= 0 {
		return err
	}
	return fmt.Errorf("%w; reconnect in %s", err, delay)
}

// IsPLCError reports whether err was returned by the controller rather than
// caused by a transport failure.
func IsPLCError(err error) bool {
	var plcErr *gos7patch.PLCError
	var s7Err *gos7patch.S7Error
	return errors.As(err, &plcErr) || errors.As(err, &s7Err)
}

func (c *client) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.lifecycleCtx.Done():
		return ErrClientClosed
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.lifecycleCtx.Done():
		return ErrClientClosed
	case <-c.operationGate:
	}
	select {
	case <-c.lifecycleCtx.Done():
		c.release()
		return ErrClientClosed
	default:
		return nil
	}
}

func (c *client) release() {
	c.operationGate <- struct{}{}
}

func (c *client) operationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	operationCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.lifecycleCtx, cancel)
	return operationCtx, func() {
		stop()
		cancel()
	}
}

func (c *client) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	c.lifecycleCancel()
	<-c.operationGate
	defer c.release()

	if c.connectionState == connectionClosed {
		return nil
	}
	c.connectionState = connectionClosed
	return c.handler.Close()
}

func (c *client) Write(ctx context.Context, addr VMAddr, value uint32) error {
	if err := addr.Validate(); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if addr.Type == Output {
		return fmt.Errorf("write %s: %w", addr, ErrReadOnlyAddress)
	}
	if err := c.acquire(ctx); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	defer c.release()
	operationCtx, cancel := c.operationContext(ctx)
	defer cancel()

	if err := c.ensureConnected(operationCtx); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	size := addr.Type.Size()
	buff := make([]byte, size)
	if addr.Type == Bit {
		if err := c.client.AGReadDB(c.dbNumber, int(addr.Byte), size, buff); err != nil {
			return c.operationFailed(err)
		}
	}
	if err := c.writeToBuffer(addr, buff, value); err != nil {
		return err
	}
	if err := c.client.AGWriteDB(c.dbNumber, int(addr.Byte), size, buff); err != nil {
		return c.operationFailed(err)
	}
	return nil
}

func (c *client) WriteMany(ctx context.Context, args ...VMAddrValue) error {
	if len(args) == 0 {
		return fmt.Errorf("failed `WriteMany`: args is empty")
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
	if err := c.acquire(ctx); err != nil {
		return fmt.Errorf("WriteMany: %w", err)
	}
	defer c.release()
	operationCtx, cancel := c.operationContext(ctx)
	defer cancel()

	if err := c.ensureConnected(operationCtx); err != nil {
		return fmt.Errorf("WriteMany: %w", err)
	}
	buff := make([]byte, size)
	if err := c.client.AGReadDB(c.dbNumber, int(start), size, buff); err != nil {
		return c.operationFailed(err)
	}
	for _, val := range args {
		offset := int(val.VMAddr.Byte - start)
		if err := c.writeToBuffer(val.VMAddr, buff[offset:], val.Value); err != nil {
			return err
		}
	}
	if err := c.client.AGWriteDB(c.dbNumber, int(start), size, buff); err != nil {
		return c.operationFailed(err)
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
	case Word:
		c.helper.SetValueAt(buff, 0, uint16(value))
	default:
		return errors.New("write: unknown data type")
	}

	return nil
}

func (c *client) Read(ctx context.Context, addr VMAddr) (uint32, error) {
	if err := addr.Validate(); err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	if err := c.acquire(ctx); err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	defer c.release()
	operationCtx, cancel := c.operationContext(ctx)
	defer cancel()

	if err := c.ensureConnected(operationCtx); err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	size := addr.Type.Size()
	buff := make([]byte, size)
	if err := c.client.AGReadDB(c.dbNumber, int(addr.Byte), size, buff); err != nil {
		return 0, c.operationFailed(err)
	}
	result, err := c.getIntFromBuffer(addr, buff)
	if err != nil {
		return 0, err
	}
	return result, nil
}

func (c *client) ReadMany(ctx context.Context, args ...VMAddr) (VMAddrValues, error) {
	return c.readMany(ctx, args...)
}

func (c *client) readMany(ctx context.Context, args ...VMAddr) (VMAddrValues, error) {
	if len(args) == 0 {
		return nil, nil
	}
	start, size, err := vmAddrRange(args)
	if err != nil {
		return nil, fmt.Errorf("ReadMany: %w", err)
	}
	if err := c.acquire(ctx); err != nil {
		return nil, fmt.Errorf("ReadMany: %w", err)
	}
	defer c.release()
	operationCtx, cancel := c.operationContext(ctx)
	defer cancel()

	if err := c.ensureConnected(operationCtx); err != nil {
		return nil, fmt.Errorf("ReadMany: %w", err)
	}
	buff := make([]byte, size)
	if err := c.readManyTo(buff, start, size); err != nil {
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

func (c *client) ReadManyTo(ctx context.Context, buff []byte, args ...VMAddr) error {
	if len(args) == 0 {
		return nil
	}
	start, size, err := vmAddrRange(args)
	if err != nil {
		return fmt.Errorf("ReadManyTo: %w", err)
	}
	if len(buff) < size {
		return fmt.Errorf("ReadManyTo: need %d bytes, but buffer only %d bytes", size, len(buff))
	}
	if err := c.acquire(ctx); err != nil {
		return fmt.Errorf("ReadManyTo: %w", err)
	}
	defer c.release()
	operationCtx, cancel := c.operationContext(ctx)
	defer cancel()

	if err := c.ensureConnected(operationCtx); err != nil {
		return fmt.Errorf("ReadManyTo: %w", err)
	}
	return c.readManyTo(buff, start, size)
}

func (c *client) readManyTo(buff []byte, start uint16, size int) error {
	if err := c.client.AGReadDB(c.dbNumber, int(start), size, buff); err != nil {
		return c.operationFailed(err)
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
	if _, _, err := vmAddrRange(addrs); err != nil {
		return nil, fmt.Errorf("stream: %w", err)
	}

	streamAddrs := append([]VMAddr(nil), addrs...)
	results := make(chan StreamResult, 1)
	streamCtx, cancel := c.operationContext(ctx)
	go func() {
		defer cancel()
		c.stream(streamCtx, interval, streamAddrs, results)
	}()
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
		data, err := c.readMany(ctx, addrs...)
		if !send(ctx, results, StreamResult{Data: data, Err: err}) {
			return
		}
		if err != nil && !c.reconnect {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func send[T any](ctx context.Context, results chan<- T, result T) bool {
	select {
	case results <- result:
		return true
	case <-ctx.Done():
		return false
	}
}
