package main

import (
	"encoding/json"
	"flag"
	"fmt"
	// "github.com/currantlabs/ble"
	// "github.com/currantlabs/ble/linux"
	"github.com/eclipse/paho.mqtt.golang"
	"github.com/thecubic/miao2go"
	// "golang.org/x/net/context"
	"log"
	"os"
	"time"
)

var (
	broker   = flag.String("broker", "tcp://localhost:1883", "MQTT broker address")
	prefix   = flag.String("prefix", "", "subscription prefix")
	topic    = flag.String("topic", "mmpackets", "subscription topic")
	clientid = flag.String("clientid", "m2g-mqs", "MQTT Client ID")
)

func main() {
	var err error
	msgTransport := make(chan mqtt.Message)
	var msgHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
		msgTransport <- msg
	}
	mqtt.ERROR = log.New(os.Stderr, "", 0)
	opts := mqtt.NewClientOptions().AddBroker(*broker).SetClientID(*clientid)
	opts.SetKeepAlive(2 * time.Second)
	opts.SetDefaultPublishHandler(msgHandler)
	opts.SetPingTimeout(1 * time.Second)

	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	fulltopic := fmt.Sprintf("%s%s", *prefix, *topic)
	if token := c.Subscribe(fulltopic, 0, nil); token.Wait() && token.Error() != nil {
		fmt.Println(token.Error())
		os.Exit(1)
	}

	for msg := range msgTransport {
		log.Printf("[%v] %v", msg.MessageID(), msg.Topic())
		var mmp miao2go.MiaoMiaoPacket
		err = json.Unmarshal(msg.Payload(), &mmp)
		if err == nil {
			mmp.Print()
			mmp.LibrePacket.Print()
		} else {
			log.Printf("err in Unmarshal: %v", err)
		}
	}

	// for i := 0; i < 5; i++ {
	// 	text := fmt.Sprintf("this is msg #%d!", i)
	// 	token := c.Publish(fulltopic, 0, false, text)
	// 	token.Wait()
	// }
	//
	// time.Sleep(6 * time.Second)
	//
	// if token := c.Unsubscribe(fulltopic); token.Wait() && token.Error() != nil {
	// 	fmt.Println(token.Error())
	// 	os.Exit(1)
	// }

	c.Disconnect(250)

	time.Sleep(1 * time.Second)
}
