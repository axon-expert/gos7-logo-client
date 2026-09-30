
# Siemens LOGO! client library

Install the dependency:

```bash
go get github.com/axon-expert/gos7-logo-client
```

Example:

```go
config := gos7logo.NewConfig("localhost")
client := gos7logo.NewClient(config)
if err := client.Connect(context.Background()); err != nil { ... }
defer client.Disconnect()

value := uint32(100)
vmAddr, err := gos7logo.NewVmAddrFromString("V94")
if err != nil { ... }

// Write a value.
if err := client.Write(vmAddr, value); err != nil { ... }

// Read a value.
result, err := client.Read(vmAddr)
if err != nil { ... }
```

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
```

`read` and `watch` accept inclusive address ranges such as `V3-V5`, `VW10-VW15`,
or the cross-byte bit range `V3.6-V4.2`.

Use `-f d` for decimal output (the default), `-f x` or `-f h` for hexadecimal,
`-f b` for binary, and `-f o` for octal.
Append an underscore, for example `-f x_`, to group hexadecimal output by bytes,
binary output by four bits, or decimal and octal output by three digits.

Run `./logo -h` for connection options, including local and LOGO! TSAP values.

## License

This library is dual-licensed under:

1. **MIT License** — permits use in open-source projects.
2. **BSD 3-Clause License** — permits use, modification, and distribution subject to its terms.

The complete license texts are available in [LICENSE](LICENSE) and
[LICENSE_GOS7](LICENSE_GOS7).
