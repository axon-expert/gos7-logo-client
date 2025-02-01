package gos7logo

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/robinson/gos7"
)

type dataType int

func (t dataType) Size() int {
	switch t {
	case Bit:
		return 2
	case Byte:
		return 1
	case Word:
		return 2
	case DWord:
		return 4
	case Real:
		return 4
	case Counter:
		return 2
	case Timer:
		return 2
	default:
		return 0
	}
}

const (
	Byte dataType = iota
	Bit
	Word
	Counter
	Timer
	DWord
	Real
)

func parseVmAddr(addr string) (int, dataType, error) {
	switch {
	case regexp.MustCompile(`V[0-9]{1,4}\.[0-7]`).MatchString(addr):
		addrSlice := strings.Split(addr[1:], ".")
		addrByte, err := strconv.Atoi(addrSlice[0])
		if err != nil {
			return 0, 0, fmt.Errorf("failed parse `byte` for `%s` from `%s`", addrSlice[0], addr)
		}
		addrBit, err := strconv.Atoi(addrSlice[1])
		if err != nil {
			return 0, 0, fmt.Errorf("failed parse `bit` for `%s` from `%s`", addrSlice[1], addr)
		}
		return (addrByte * 8) + addrBit, Bit, nil
	case regexp.MustCompile(`V[0-9]+`).MatchString(addr):
		start, err := strconv.Atoi(addr[1:])
		if err != nil {
			return 0, 0, fmt.Errorf("failed parse start address `Byte` for `%s` from `%s`", addr[1:], addr)
		}
		return start, Byte, nil
	case regexp.MustCompile(`VW[0-9]+`).MatchString(addr):
		start, err := strconv.Atoi(addr[2:])
		if err != nil {
			return 0, 0, fmt.Errorf("failed parse start address `Word` for `%s` from `%s`", addr[2:], addr)
		}
		return start, Word, nil
	case regexp.MustCompile(`VD[0-9]+`).MatchString(addr):
		start, err := strconv.Atoi(addr[2:])
		if err != nil {
			return 0, 0, fmt.Errorf("failed parse start address `DWord` for `%s` from `%s`", addr[2:], addr)
		}
		return start, DWord, nil
	}

	return 0, 0, errors.New("unknown address format")
}

type ConnectOpt struct {
	Addr string
	Rack int
	Slot int
}

type Client interface {
	Read(vmAddr string) (int, error)
	Write(vmAddr string, value int) error
	Disconnect() error
}

type client struct {
	helper   gos7.Helper
	client   gos7.Client
	handler  *gos7.TCPClientHandler
	area     string
	dbNumber int
}

func NewClient(opt *ConnectOpt) (*client, error) {
	handler := gos7.NewTCPClientHandler(opt.Addr, opt.Rack, opt.Slot)
	if err := handler.Connect(); err != nil {
		return nil, err
	}
	return &client{
		area: "DB", dbNumber: 1,
		client:  gos7.NewClient(handler),
		handler: handler}, nil
}

func (c *client) Read(vmAddr string) (int, error) {
	start, dataType, err := parseVmAddr(vmAddr)
	if err != nil {
		return 0, err
	}
	size := dataType.Size()
	buff := make([]byte, size)
	if err := c.client.AGReadDB(c.dbNumber, start, size, buff); err != nil {
		return 0, err
	}
	result, err := c.getIntFromBuffer(dataType, buff)
	if err != nil {
		return 0, err
	}
	return result, nil
}

func (c *client) Write(vmAddr string, value int) error {
	start, dataType, err := parseVmAddr(vmAddr)
	if err != nil {
		return err
	}
	size := dataType.Size()
	buff := make([]byte, size)
	if err := c.writeToBuffer(dataType, buff, value); err != nil {
		return err
	}
	if err := c.client.AGWriteDB(c.dbNumber, start, size, buff); err != nil {
		return err
	}
	return nil
}

func (c *client) writeToBuffer(dataType dataType, buff []byte, value int) error {
	switch dataType {
	case Bit:
		c.helper.SetValueAt(buff, 0, int16(value))
	case Byte:
		c.helper.SetValueAt(buff, 0, int8(value))
	case Word:
		c.helper.SetValueAt(buff, 0, int16(value))
	case DWord:
		c.helper.SetValueAt(buff, 0, int32(value))
	case Real:
		c.helper.SetValueAt(buff, 0, float32(value))
	case Counter:
		c.helper.SetValueAt(buff, 0, int16(value))
	case Timer:
		c.helper.SetValueAt(buff, 0, int16(value))
	default:
		return errors.New("write: unknown data type")
	}

	return nil
}
func (c *client) getIntFromBuffer(dataType dataType, buff []byte) (int, error) {
	switch dataType {
	case Bit:
		var result int16
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	case Byte:
		var result int8
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	case Word:
		var result int16
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	case DWord:
		var result int32
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	case Real:
		var result float32
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	case Counter:
		var result int16
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	case Timer:
		var result int16
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	}

	return 0, errors.New("write: unknown data type")
}

func (c *client) Disconnect() error {
	return c.handler.Close()
}
