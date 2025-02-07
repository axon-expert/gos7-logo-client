
# Библиотека для работы с Siemens Logo

Установка зависимости
```
go get github.com/axon-expert/gos7-logo-client
```

Пример использования:
```go
client, err := gos7logo.NewClient(gos7logo.ConnectOpt{
    Addr: "localhost:102",
    Rack: 0,  Slot: 1,
})
if err != nil { ... }
defer client.Disconnect()

value := 100
vmAddr, err := gos7logo.NewVmAddrFromString("V94") 
// you can specify a bit `gos7logo.NewVmAddr("V", 2, 1)`
// or `gos7logo.NewVmAddr("V", 94)`
if err != nil { ... }

// Запись значения
if err := client.Write(vmAddr, value); err != nil { ... }

// Чтение значения
result, err := client.Read(vmAddr)
if err != nil { ... }
```

## Лицензия

Данная библиотека распространяется под двойной лицензией:

1. **MIT License** — свободная лицензия, позволяющая использовать библиотеку в открытых проектах.
2. **BSD 3-Clause License** — для свободного использования, модифицирования и распространения программного обеспечения при соблюдении определённых условий.

Полный текст лицензий доступен в файлах [LICENSE](LICENSE), [LICENSE GOS7](LICENSE_GOS7).