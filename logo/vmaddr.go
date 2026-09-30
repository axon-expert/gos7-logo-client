package gos7logo

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type DataType int

const (
	Byte DataType = iota
	Bit
	Word
	Counter
	Timer
	DWord
	Real
	Output
)

const (
	outputByteStart = uint32(1064)
	outputByteEnd   = uint32(1071)
)

func (t DataType) Size() int {
	switch t {
	case Bit, Byte, Output:
		return 1
	case Word, Counter, Timer:
		return 2
	case DWord, Real:
		return 4
	default:
		return 0
	}
}

func (t DataType) String() string {
	switch t {
	case Byte:
		return "V"
	case Bit:
		return "V"
	case Word, Counter, Timer:
		return "VW"
	case DWord:
		return "VD"
	case Output:
		return "Q"
	}
	return "V"
}

func parseTypeByVmAddr(addr string) (DataType, error) {
	switch {
	case regexp.MustCompile(`^V[0-9]+\.[0-7]$`).MatchString(addr):
		return Bit, nil
	case regexp.MustCompile(`^V[0-9]+$`).MatchString(addr):
		return Byte, nil
	case regexp.MustCompile(`^VW[0-9]+$`).MatchString(addr):
		return Word, nil
	case regexp.MustCompile(`^VD[0-9]+$`).MatchString(addr):
		return DWord, nil
	case regexp.MustCompile(`^Q[0-9]+$`).MatchString(addr):
		return Output, nil
	}

	return 0, errors.New("unknown address format")
}

type VmAddr struct {
	Type DataType
	Byte uint32
	Bit  uint8
}

func NewVmAddr(t DataType, byteAddr uint32, bit uint8) (VmAddr, error) {
	addr := VmAddr{Type: t, Bit: bit, Byte: byteAddr}
	if err := addr.Validate(); err != nil {
		return VmAddr{}, err
	}
	return addr, nil
}

func MustNewVmAddr(t DataType, byteAddr uint32, bit uint8) VmAddr {
	addr, err := NewVmAddr(t, byteAddr, bit)
	if err != nil {
		panic(err)
	}
	return addr
}

func (a VmAddr) Validate() error {
	if a.Type.Size() == 0 {
		return fmt.Errorf("unknown data type: %d", a.Type)
	}
	if a.Type == Bit || a.Type == Output {
		if a.Bit > 7 {
			return fmt.Errorf("bit index must be between 0 and 7: %d", a.Bit)
		}
		if a.Type == Output && (a.Byte < outputByteStart || a.Byte > outputByteEnd) {
			return fmt.Errorf("output address must be between Q1 and Q64")
		}
		return nil
	}
	if a.Bit != 0 {
		return fmt.Errorf("bit index must be zero for data type %v: %d", a.Type, a.Bit)
	}
	return nil
}

func NewVmAddrFromString(addr string) (VmAddr, error) {
	a := VmAddr{}
	if err := a.UnmarshalText([]byte(addr)); err != nil {
		return a, err
	}
	return a, nil
}

func MustNewVmAddrFromString(addr string) VmAddr {
	parsed, err := NewVmAddrFromString(addr)
	if err != nil {
		panic(err)
	}
	return parsed
}

func (addr VmAddr) MarshalText() ([]byte, error) {
	if addr.Type == Output {
		output := (addr.Byte-outputByteStart)*8 + uint32(addr.Bit) + 1
		return fmt.Appendf(nil, "Q%d", output), nil
	}
	if addr.Type == Bit {
		return fmt.Appendf(nil, "%s%d.%d", addr.Type.String(), addr.Byte, addr.Bit), nil
	}
	return fmt.Appendf(nil, "%s%d", addr.Type.String(), addr.Byte), nil
}

func (addr VmAddr) String() string {
	raw, _ := addr.MarshalText()
	return string(raw)
}

func (a *VmAddr) UnmarshalText(raw []byte) error {
	addr := string(raw)
	addrType, err := parseTypeByVmAddr(addr)
	if err != nil {
		return fmt.Errorf("failed parse data type: %s", err)
	}
	if addrType == Output {
		output, err := strconv.ParseUint(addr[1:], 10, 8)
		if err != nil {
			return fmt.Errorf("invalid output address: %w", err)
		}
		if output < 1 || output > 64 {
			return errors.New("output address must be between Q1 and Q64")
		}
		output--
		*a = VmAddr{
			Type: Output,
			Byte: outputByteStart + uint32(output/8),
			Bit:  uint8(output % 8),
		}
		return nil
	}

	addrSlice := strings.Split(addr, ".")
	prefixLength := 1
	if addrType == Word || addrType == DWord {
		prefixLength = 2
	}
	byteAddr, err := strconv.ParseUint(addrSlice[0][prefixLength:], 10, 32)
	if err != nil {
		return fmt.Errorf("invalid byte address: %w", err)
	}
	parsed := VmAddr{Type: addrType, Byte: uint32(byteAddr)}
	if addrType == Bit {
		bitAddr, err := strconv.ParseUint(addrSlice[1], 10, 8)
		if err != nil {
			return fmt.Errorf("invalid bit address: %w", err)
		}
		parsed.Bit = uint8(bitAddr)
	}
	if err := parsed.Validate(); err != nil {
		return err
	}
	*a = parsed
	return nil
}

type VmAddrValue struct {
	VmAddr VmAddr
	Value  uint32
}

type VmAddrValues []VmAddrValue

func (values VmAddrValues) At(index int) (VmAddrValue, bool) {
	if index < 0 || index >= len(values) {
		return VmAddrValue{}, false
	}
	return values[index], true
}

func (values VmAddrValues) Get(addr VmAddr) (uint32, bool) {
	for _, value := range values {
		if value.VmAddr == addr {
			return value.Value, true
		}
	}
	return 0, false
}

func compareVmAddrByte(lhs, rhs VmAddr) int {
	return cmp.Compare(lhs.Byte, rhs.Byte)
}
