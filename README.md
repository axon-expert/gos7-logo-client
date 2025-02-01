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
vmAddr := "V94"

// Запись значения
if err := client.Write(vmAddr, value); err != nil { ... }

// Чтение значения
result, err := client.Read(vmAddr)
if err != nil { ... }
```