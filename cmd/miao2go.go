package main

import (
	"flag"
	// "fmt"
	"github.com/currantlabs/ble"
	"github.com/currantlabs/ble/linux"
	"github.com/thecubic/miao2go"
	"golang.org/x/net/context"
	"log"
	"time"
)

var (
	timeout    = flag.Duration("timeout", 60*time.Second, "timeout")
	miao       = flag.String("miao", "", "address of the miaomiao")
	dup        = flag.Bool("dup", true, "allow duplicate reported")
	clientChar = ble.MustParse("00002902-0000-1000-8000-00805f9b34fb")
)

func main() {
	flag.Parse()

	if len(*miao) == 0 {
		log.Fatalf("must pass miao")
	}
	d, err := linux.NewDevice()
	if err != nil {
		log.Fatalf("can't new device : %s", err)
	}
	ble.SetDefaultDevice(d)

	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), *timeout))

	log.Printf("connecting to %v", *miao)
	filter := func(adv ble.Advertisement) bool {
		if len(adv.LocalName()) > 0 {
			if adv.LocalName() == "miaomiao" {
				log.Printf("found a miao: %v", adv.Address().String())
			}
		}
		return adv.Address().String() == *miao
	}
	cln, err := ble.Connect(ctx, filter)
	if err != nil {
		log.Fatalf("couldn't connect to %v: %v", miao, err)
	} else {
		log.Printf("connected to %v", cln.Address())
	}

	conend := make(chan struct{})

	go func() {
		<-cln.Disconnected()
		log.Printf("disconnected from %v", cln.Address())
		close(conend)
	}()

	miao, err := miao2go.AttachBTLE(cln)

	if err != nil {
		log.Fatalf("couldn't get Miao descriptor: %v", err)
	}
	log.Printf("miao: %v\n", miao)

	err = miao.Subscribe()
	if err != nil {
		log.Fatalf("couldn't sub/unsub Miao: %v", err)
	}

	mr, err := miao.MiaoResponse()
	if err != nil {
		log.Fatalf("mr error: %v", err)
	}

	switch mr.Type {
	case miao2go.MPLibre:
		log.Printf("Libre response: %v", mr.Data)
		mmp := miao2go.CreateMiaoMiaoPacket(mr.Data)
		log.Printf("mmp: %v", mmp)
	case miao2go.MPNoSensor:
		log.Printf("No Sensor")
	case miao2go.MPNewSensor:
		log.Printf("New Sensor")
	}

	// TODO: do something with the packet
	cln.CancelConnection()
	<-conend
}
