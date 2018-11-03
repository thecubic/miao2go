package main

// m2g-accept: accept new sensor (when relevant)

import (
	"flag"
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
	nocheck = flag.Bool("nocheck", false, "don't check for NewSensor condition")
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

	var mls miao2go.MiaoDeviceState
	newSensorMode := false
	mls, err = miao.MiaoLibreStatus()
	if err != nil {
		log.Printf("couldn't get new status: %v", err)
	} else {
		switch mls {
		case miao2go.MPDeclared:
			log.Printf("miaomiao: unknown")
		case miao2go.MPLibre:
			log.Printf("miaomiao: reporting mode")
		case miao2go.MPNoSensor:
			log.Printf("miaomiao: no sensor")
		case miao2go.MPNewSensor:
			log.Printf("miaomiao: new sensor")
			newSensorMode = true
		}
	}

	if !*nocheck && !newSensorMode {
		log.Fatalf("check failed: sensor not in new sensor mode")
	}

	log.Printf("accepting sensor...")

	err = miao.AcceptNewSensor()
	if err != nil {
		log.Printf("couldn't accept new sensor: %v", err)
	}

	mls, err = miao.MiaoLibreStatus()
	if err != nil {
		log.Printf("couldn't get new status: %v", err)
	} else {
		switch mls {
		case miao2go.MPDeclared:
			log.Printf("miaomiao: unknown")
		case miao2go.MPLibre:
			log.Printf("miaomiao: reporting mode")
		case miao2go.MPNoSensor:
			log.Printf("miaomiao: no sensor")
		case miao2go.MPNewSensor:
			log.Printf("miaomiao: new sensor")
		}
	}
	cln.CancelConnection()
}
