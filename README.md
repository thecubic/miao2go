# miao2go
miaomiao BLE driver in Go

## example


```
$ go build cmd/miao2go.go && sudo chown root miao2go && sudo chmod 4770 miao2go && ./miao2go --miao aa:aa:aa:aa:aa:aa
2018/09/25 10:55:42 connecting to aa:aa:aa:aa:aa:aa
2018/09/25 10:55:43 found a miao: bb:bb:bb:bb:bb:bb
2018/09/25 10:55:43 found a miao: bb:bb:bb:bb:bb:bb
2018/09/25 10:55:43 found a miao: aa:aa:aa:aa:aa:aa
2018/09/25 10:55:43 found a miao: aa:aa:aa:aa:aa:aa
2018/09/25 10:55:44 connected to aa:aa:aa:aa:aa:aa

2018/09/25 10:55:44 miao: &{0xc420178000 0xc4200b0400 0xc420188210 0xc4201960b0 0xc4200ba7e0 1 0xc42019a000}
2018/09/25 10:55:45 MSSubscribed -> MSBeingNotified
2018/09/25 10:55:45 receiving encapsulated Libre packet
2018/09/25 10:55:45 Libre response
2018/09/25 10:55:45 disconnected from aa:aa:aa:aa:aa:aa
```
