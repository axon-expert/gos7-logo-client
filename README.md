
# Siemens LOGO! client library

Install the dependency:

```bash
go get github.com/axon-expert/gos7-logo-client
```

Example:

```go
config := gos7logo.NewConfig("localhost")
config.Reconnect = true // Optional.
config.InitialReconnectDelay = 250 * time.Millisecond
config.MaxReconnectDelay = 5 * time.Second
client := gos7logo.NewClient(config)
defer client.Close()
ctx := context.Background()

value := uint32(100)
vmAddr, err := gos7logo.NewVMAddrFromString("V94")
if err != nil { ... }

// Write a value.
if err := client.Write(ctx, vmAddr, value); err != nil { ... }

// Read a value.
result, err := client.Read(ctx, vmAddr)
if err != nil { ... }
```

The client connects lazily on the first operation. When `Reconnect` is
enabled, an operation that detects a broken connection still returns its error;
later operations reconnect automatically with exponential backoff.

## Console client

Build the simple command-line client:

```sh
go build -o logo ./cmd/logo
```

Read, write, or continuously watch VM addresses:

```sh
./logo -host 192.168.0.10 -port 102 read V1 VW2 VD4
./logo -host 192.168.0.10 -port 102 write V1 42 V2.3 1
./logo -host 192.168.0.10 -port 102 watch -interval 500ms V1 VW2
./logo -host 192.168.0.10 -port 102 -retry watch V1 VW2
./logo -host 192.168.0.10 -port 102 read Q1-Q8
```

`read` and `watch` accept inclusive address ranges such as `V3-V5`, `VW10-VW15`,
or the cross-byte bit range `V3.6-V4.2`.

Variable-memory addresses are limited to `V0.0` through `V850.7`. Multi-byte
values must also end within byte 850, so the last word is `VW849` and the last
double word is `VD847`; see the
[LOGO!Soft Comfort VM address restrictions](https://cache.industry.siemens.com/dl/files/852/109768852/att_990434/v1/Help_en-US.pdf#page=116).

Digital outputs on LOGO! 0BA8 can be addressed as `Q1` through `Q64`. These
names map to the controller's output VM range starting at `V1064.0`; see
[LOGO!Soft Comfort V8.2.1 Operating Instructions, page 119](https://cache.industry.siemens.com/dl/files/852/109768852/att_990434/v1/Help_en-US.pdf#page=119).
Q addresses are read-only; use a writable VM bit and LOGO! program logic to
control a physical output remotely.

Use `-f d` for decimal output (the default), `-f x` or `-f h` for hexadecimal,
`-f b` for binary, and `-f o` for octal.
Append an underscore, for example `-f x_`, to group hexadecimal output by bytes,
binary output by four bits, or decimal and octal output by three digits.

Use the global `-retry` option to retry `read` and `write` after connection
errors, or to reconnect after polling errors while watching. Retries continue
until the operation succeeds or the command is cancelled.

Run `./logo -h` for connection options, including local and LOGO! TSAP values.

## License

This library is dual-licensed under:

1. **MIT License** — permits use in open-source projects.
2. **BSD 3-Clause License** — permits use, modification, and distribution subject to its terms.

The complete license texts are available in [LICENSE](LICENSE) and
[LICENSE_GOS7](LICENSE_GOS7).
