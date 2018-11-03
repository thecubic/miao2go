package main

// m2g-mqp: read transciever and MQ publish measurements

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/currantlabs/ble"
	"github.com/currantlabs/ble/linux"
	"github.com/eclipse/paho.mqtt.golang"
	"github.com/thecubic/miao2go"
	"golang.org/x/net/context"
	"log"
	"os"
	"time"
)

var (
	timeout  = flag.Duration("timeout", 60*time.Second, "timeout")
	miao     = flag.String("miao", "", "address of the miaomiao")
	broker   = flag.String("broker", "tcp://localhost:1883", "MQTT broker address")
	prefix   = flag.String("prefix", "", "subscription prefix")
	topic    = flag.String("topic", "mmpackets", "subscription topic")
	clientid = flag.String("clientid", "m2g-mqp", "MQTT Client ID")
	once     = flag.Bool("once", false, "don't continue after first read")
	print    = flag.Bool("print", false, "print out packet details")
	mqdebug  = flag.Bool("mqdebug", false, "MQ debugging output")
)

func main() {
	flag.Parse()
	if len(*miao) == 0 {
		log.Fatalf("must pass miao")
	}
	if *mqdebug {
		mqtt.DEBUG = log.New(os.Stderr, "", 0)
	}
	mqtt.ERROR = log.New(os.Stderr, "", 0)
	opts := mqtt.NewClientOptions().AddBroker(*broker).SetClientID(*clientid)
	opts.SetKeepAlive(2 * time.Second)
	opts.SetPingTimeout(1 * time.Second)
	mq := mqtt.NewClient(opts)
	if token := mq.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	fulltopic := fmt.Sprintf("%s%s", *prefix, *topic)
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
		pkt, err := miao.ReadSensor()
		if err == nil {
			if *print {
				pkt.Print()
				pkt.LibrePacket.Print()
				fmt.Printf("packet captured in %v\n", pkt.EndTime.Sub(pkt.StartTime))
				// fmt.Printf("JSONed packet created, len %v\n", len(json))
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
			json, err := json.Marshal(pkt)
			if err == nil {
				fmt.Printf("packet captured in %v\n", pkt.EndTime.Sub(pkt.StartTime))
				fmt.Printf("JSONed packet created, len %v\n", len(json))
				fmt.Printf("-> %v/%v\n", *broker, fulltopic)
				token := mq.Publish(fulltopic, 0, false, json)
				token.Wait()
				fmt.Printf("published (err %v)\n", token.Error())
			} else {
				log.Printf("error in read attempt: %v", err)
			}
			fmt.Printf("next data emission scheduled for: %v\n", miao.NextEmit)
		}
	}
	cln.CancelConnection()
}
