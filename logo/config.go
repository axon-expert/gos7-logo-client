package gos7logo

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type TSAP uint16

func (t TSAP) String() string {
	return fmt.Sprintf("%02x.%02x", byte(t>>8), byte(t))
}

func (t TSAP) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *TSAP) UnmarshalText(text []byte) error {
	raw := string(text)
	if strings.Contains(raw, ".") {
		parts := strings.Split(raw, ".")
		if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
			return fmt.Errorf("invalid dotted TSAP %q: expected XX.XX", raw)
		}
		high, err := strconv.ParseUint(parts[0], 16, 8)
		if err != nil {
			return fmt.Errorf("invalid TSAP high byte %q: %w", parts[0], err)
		}
		low, err := strconv.ParseUint(parts[1], 16, 8)
		if err != nil {
			return fmt.Errorf("invalid TSAP low byte %q: %w", parts[1], err)
		}
		*t = TSAP(high<<8 | low)
		return nil
	}

	value, err := strconv.ParseUint(raw, 0, 16)
	if err != nil {
		return fmt.Errorf("invalid TSAP %q: %w", raw, err)
	}
	*t = TSAP(value)
	return nil
}

func (t *TSAP) UnmarshalJSON(data []byte) error {
	raw := string(data)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		value, err := strconv.Unquote(raw)
		if err != nil {
			return fmt.Errorf("invalid quoted TSAP: %w", err)
		}
		return t.UnmarshalText([]byte(value))
	}
	return t.UnmarshalText(data)
}

const (
	defaultLocalTSAP  TSAP = 0x1000
	defaultRemoteTSAP TSAP = 0x2000
	defaultPort            = 102
)

type Config struct {
	Host       string
	Port       uint16
	LocalTSAP  TSAP
	RemoteTSAP TSAP
}

func NewConfig(host string) Config {
	return Config{
		Host:       host,
		Port:       defaultPort,
		LocalTSAP:  defaultLocalTSAP,
		RemoteTSAP: defaultRemoteTSAP,
	}
}

func (c Config) endpoint() string {
	return net.JoinHostPort(c.Host, strconv.FormatUint(uint64(c.Port), 10))
}
