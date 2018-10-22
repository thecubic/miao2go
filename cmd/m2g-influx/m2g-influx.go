package main

// miao2go: accept new sensor (when relevant)

import (
	"flag"
	"fmt"
	"github.com/currantlabs/ble"
	"github.com/currantlabs/ble/linux"
	"github.com/thecubic/miao2go"
	"golang.org/x/net/context"
	"log"
	"time"
)

var (
	timeout = flag.Duration("timeout", 60*time.Second, "timeout")
	miao    = flag.String("miao", "", "address of the miaomiao")
	check   = flag.Bool("check", true, "check for NewSensor condition")
	once    = flag.Bool("once", false, "don't continue after first read")
	print   = flag.Bool("print", false, "print out packet details")
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

	go func() {
		<-cln.Disconnected()
		log.Printf("disconnected from %v", cln.Address())
	}()

	miao, err := miao2go.AttachBTLE(cln)
	if err != nil {
		log.Fatalf("couldn't get Miao descriptor: %v", err)
	}

	if *once {
		reading, err := miao.ReadSensor()
		if err == nil {
			if *print {
				reading.Print()
				reading.LibrePacket.Print()
			}
		} else {
			log.Printf("error in read attempt: %v", err)
		}
	} else {
		emitter := miao.ReadingEmitter()
		for pkt := range emitter {
			if *print {
				pkt.Print()
				pkt.LibrePacket.Print()
			}
			json, err := pkt.ToJSON()
			if err == nil {
				fmt.Printf("packet captured in %v\n", pkt.EndTime.Sub(pkt.StartTime))
				fmt.Printf("JSONed packet created, len %v\n", len(json))
			} else {
				log.Printf("error in read attempt: %v", err)
			}
			fmt.Printf("next data emission scheduled for: %v\n", miao.NextEmit)
		}
	}
	cln.CancelConnection()
}
