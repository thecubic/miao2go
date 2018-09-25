package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"golang.org/x/net/context"

	"github.com/currantlabs/ble"
	"github.com/currantlabs/ble/linux"
)

var (
	timeout = flag.Duration("timeout", 60*time.Second, "timeout")
	miao    = flag.String("miao", "", "address of the miaomiao")
	dup     = flag.Bool("dup", true, "allow duplicate reported")

	clientChar = ble.MustParse("00002902-0000-1000-8000-00805f9b34fb")
	nrfData    = ble.MustParse("6E400001-B5A3-F393-E0A9-E50E24DCCA9E")
	nrfRecv    = ble.MustParse("6E400002-B5A3-F393-E0A9-E50E24DCCA9E")
	nrfXmit    = ble.MustParse("6E400003-B5A3-F393-E0A9-E50E24DCCA9E")
)

type MiaoState int

const (
	MSDeclared      MiaoState = 0
	MSConnected     MiaoState = 1
	MSSubscribed    MiaoState = 2
	MSBeingNotified MiaoState = 3
)

type MiaoPacketType byte

const (
	MPDeclared  MiaoPacketType = 0x00
	MPLibre     MiaoPacketType = 0x28
	MPNewSensor MiaoPacketType = 0x32
	MPNoSensor  MiaoPacketType = 0x34
)

const encapsulatedEnd = 0x29

type gattResponsePacket struct {
	data []byte
}

type miaoResponsePacket struct {
	ptype        MiaoPacketType
	data         [363]byte
	sensorPacket *libreResponsePacket
}

type libreResponsePacket struct {
	data []byte
}

type legitConnectedMiao struct {
	client         ble.Client
	nrfDataService *ble.Service
	nrfRecvChar    *ble.Characteristic
	nrfXmitChar    *ble.Characteristic
	clientDesc     *ble.Descriptor
	state          MiaoState
	datachan       chan gattResponsePacket
}

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

	miao, err := getMiaoDescriptor(cln)
	if err != nil {
		log.Fatalf("couldn't get Miao descriptor: %v", err)
	}
	log.Printf("miao: %v\n", miao)

	err = miao.subscribe()
	if err != nil {
		log.Fatalf("couldn't sub/unsub Miao: %v", err)
	}

	mr, err := miao.miaoResponse()
	if err != nil {
		log.Fatalf("mr error: %v", err)
	}

	switch mr.ptype {
	case MPLibre:
		log.Printf("Libre response")
	case MPNoSensor:
		log.Printf("No Sensor")
	case MPNewSensor:
		log.Printf("New Sensor")
	}

	// TODO: do something with the packet
	cln.CancelConnection()
	<-conend
}

func getMiaoDescriptor(blec ble.Client) (*legitConnectedMiao, error) {
	var err error
	var nrfDataService *ble.Service
	var nrfDataRecv *ble.Characteristic
	var nrfDataXmit *ble.Characteristic
	var miaoClientDesc *ble.Descriptor
	blep, err := blec.DiscoverProfile(true)
	if err != nil {
		log.Fatalf("couldn't fetch BLE profile")
	}
	for _, s := range blep.Services {
		if !s.UUID.Equal(nrfData) {
			nrfDataService = s
			// only care about miao data service
			continue
		}
		for _, c := range s.Characteristics {
			if c.UUID.Equal(nrfRecv) {
				nrfDataRecv = c
			} else if c.UUID.Equal(nrfXmit) {
				nrfDataXmit = c
			} else {
				// DGAF
				continue
			}
			for _, d := range c.Descriptors {
				if c.UUID.Equal(nrfXmit) && d.UUID.Equal(ble.ClientCharacteristicConfigUUID) {
					miaoClientDesc = d
				}
			}

		}
		fmt.Printf("\n")
	}

	if nrfDataService == nil {
		return nil, fmt.Errorf("nrfDataService missing")
	} else if nrfDataRecv == nil {
		return nil, fmt.Errorf("nrfDataRecv missing")
	} else if nrfDataXmit == nil {
		return nil, fmt.Errorf("nrfDataXmit missing")
	} else if miaoClientDesc == nil {
		return nil, fmt.Errorf("miaoClientDesc missing")
	}
	// we're in business!
	return &legitConnectedMiao{blec, nrfDataService, nrfDataRecv, nrfDataXmit,
		miaoClientDesc, MSConnected, make(chan gattResponsePacket)}, nil
}

func (lcm *legitConnectedMiao) gattDataCallback(data []byte) {
	if lcm.state == MSSubscribed {
		log.Printf("MSSubscribed -> MSBeingNotified")
		lcm.state = MSBeingNotified
	}
	lcm.datachan <- gattResponsePacket{data}
}

func (lcm *legitConnectedMiao) miaoResponse() (*miaoResponsePacket, error) {
	var response *miaoResponsePacket
	var packetData [363]byte
	packetOffset := 0
	packetFinished := false
	response = &miaoResponsePacket{MPDeclared, packetData, nil}
	for packetFinished == false {
		gattpacket, ok := <-lcm.datachan
		gattpacketlength := len(gattpacket.data)
		if !ok {
			return nil, fmt.Errorf("datachan hangup")
		}
		copied := copy(packetData[packetOffset:packetOffset+gattpacketlength], gattpacket.data)
		if packetOffset == 0 && copied >= 1 {
			switch packetData[0] {
			case byte(MPNoSensor):
				response.ptype = MPNoSensor
				packetFinished = true
			case byte(MPNewSensor):
				response.ptype = MPNewSensor
				packetFinished = true
			case byte(MPLibre):
				log.Printf("receiving encapsulated Libre packet")
				response.ptype = MPLibre
				packetFinished = false
			}
		}
		packetOffset += copied
		if packetOffset == len(packetData) {
			packetFinished = true
		}
	}
	lcm.state = MSSubscribed
	return response, nil
}

func (lcm *legitConnectedMiao) subscribe() error {
	var err error
	if err = lcm.client.Subscribe(lcm.nrfXmitChar, false, lcm.gattDataCallback); err != nil {
		log.Fatalf("XMIT subscribe failed: %s", err)
	}
	lcm.state = MSSubscribed
	err = lcm.client.WriteDescriptor(lcm.clientDesc, []byte{0x01, 0x00})
	if err != nil {
		return fmt.Errorf("error in first write: %v", err)
	}
	err = lcm.client.WriteCharacteristic(lcm.nrfRecvChar, []byte{0xf0}, false)
	if err != nil {
		return fmt.Errorf("error in second write: %v", err)
	}
	return nil
}
