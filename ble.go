package miao2go

import (
	"fmt"
	"github.com/currantlabs/ble"
	"log"
	"time"
)

var (
	nrfData = ble.MustParse("6E400001-B5A3-F393-E0A9-E50E24DCCA9E")
	nrfRecv = ble.MustParse("6E400002-B5A3-F393-E0A9-E50E24DCCA9E")
	nrfXmit = ble.MustParse("6E400003-B5A3-F393-E0A9-E50E24DCCA9E")
)

// MiaoBluetoothState represents the percieved BLE state of the device
type MiaoBluetoothState int

// BLE states
const (
	MSDeclared      MiaoBluetoothState = 0
	MSConnected     MiaoBluetoothState = 1
	MSSubscribed    MiaoBluetoothState = 2
	MSBeingNotified MiaoBluetoothState = 3
)

// gattResponsePacket is a direct representation of a BLE read
type gattResponsePacket struct {
	data []byte
	time time.Time
}

// ConnectedMiao represents a BLE connection to a miaomiao
type ConnectedMiao struct {
	client         ble.Client
	nrfDataService *ble.Service
	nrfRecvChar    *ble.Characteristic
	nrfXmitChar    *ble.Characteristic
	clientDesc     *ble.Descriptor
	BtState        MiaoBluetoothState
	DevState       MiaoDeviceState
	datachan       chan gattResponsePacket
	LastEmit       time.Time
	NextEmit       time.Time
	emitInterval   time.Duration
}

// AttachBTLE creates a connection descriptor for a miaomiao based on input
// of a legitimate BLE-layer connected device.  It will fail if you give it
// a BT mouse or whatever
func AttachBTLE(blec ble.Client) (*ConnectedMiao, error) {
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
	return &ConnectedMiao{
		blec,
		nrfDataService,
		nrfDataRecv,
		nrfDataXmit,
		miaoClientDesc,
		MSConnected,
		MPDeclared,
		make(chan gattResponsePacket),
		zeroTime,
		zeroTime,
		zeroDuration,
	}, nil
}
