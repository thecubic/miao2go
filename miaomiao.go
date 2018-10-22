package miao2go

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/currantlabs/ble"
	"log"
	"time"
)

var (
	zeroTime     = time.Time{}
	zeroDuration = time.Duration(0)
	nrfData      = ble.MustParse("6E400001-B5A3-F393-E0A9-E50E24DCCA9E")
	nrfRecv      = ble.MustParse("6E400002-B5A3-F393-E0A9-E50E24DCCA9E")
	nrfXmit      = ble.MustParse("6E400003-B5A3-F393-E0A9-E50E24DCCA9E")
)

type MiaoBluetoothState int

const (
	MSDeclared      MiaoBluetoothState = 0
	MSConnected     MiaoBluetoothState = 1
	MSSubscribed    MiaoBluetoothState = 2
	MSBeingNotified MiaoBluetoothState = 3
)

type MiaoDeviceState byte

const (
	MPDeclared  MiaoDeviceState = 0x00
	MPLibre     MiaoDeviceState = 0x28
	MPNewSensor MiaoDeviceState = 0x32
	MPNoSensor  MiaoDeviceState = 0x34
)

const encapsulatedEnd = 0x29

type gattResponsePacket struct {
	data []byte
	time time.Time
}

type MiaoResponsePacket struct {
	Type         MiaoDeviceState
	Data         [363]byte
	SensorPacket *LibreResponsePacket
	StartTime    time.Time
	EndTime      time.Time
}

type LibreResponsePacket struct {
	Data []byte
}

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

// MiaoMiaoPacket is a deserialized device reading inclusive of a LibrePacket
type MiaoMiaoPacket struct {
	Data              [363]byte
	PktLength         uint16
	SerialNumber      string
	FimrwareVersion   uint16
	HardwareVersion   uint16
	BatteryPercentage uint8
	StartTime         time.Time
	EndTime           time.Time
	LibrePacket       *LibrePacket
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

// gattDataCallback handles the trigger of data callback and shuffles said data
// to the objects data channel for deserialization elsewhere
func (lcm *ConnectedMiao) gattDataCallback(data []byte) {
	if lcm.BtState == MSSubscribed {
		// log.Printf("MSSubscribed -> MSBeingNotified")
		lcm.BtState = MSBeingNotified
		// log.Printf("lastEmit: %v", lcm.LastEmit)
		lcm.LastEmit = time.Now()
		// log.Printf("thisEmit: %v", lcm.LastEmit)
	}
	lcm.datachan <- gattResponsePacket{data, time.Now()}
}

// MiaoResponse reads an active BTLE datastream to a packet structure
// that only represents the thin layer of the device itself, and must
// be sent to other functions ()
func (lcm *ConnectedMiao) MiaoResponse() (*MiaoResponsePacket, error) {
	var response *MiaoResponsePacket
	var packetData [363]byte
	packetOffset := 0
	packetFinished := false
	response = &MiaoResponsePacket{MPDeclared, packetData, nil, lcm.LastEmit, time.Time{}}
	for packetFinished == false {
		gattpacket, ok := <-lcm.datachan
		if response.StartTime.IsZero() {
			response.StartTime = gattpacket.time
		}
		gattpacketlength := len(gattpacket.data)
		if !ok {
			return nil, fmt.Errorf("datachan hangup")
		}
		// log.Printf("recv'd from %v - %v", packetOffset, packetOffset+gattpacketlength)
		copied := copy(response.Data[packetOffset:packetOffset+gattpacketlength], gattpacket.data)
		if packetOffset == 0 && copied >= 1 {
			switch response.Data[0] {
			case byte(MPNoSensor):
				response.Type = MPNoSensor
				lcm.DevState = MPNoSensor
				packetFinished = true
			case byte(MPNewSensor):
				response.Type = MPNewSensor
				lcm.DevState = MPNewSensor
				packetFinished = true
			case byte(MPLibre):
				response.Type = MPLibre
				lcm.DevState = MPLibre
				packetFinished = false
			}
		}
		packetOffset += copied
		if packetOffset == len(response.Data) {
			// log.Printf("packetFinished")
			packetFinished = true
			response.EndTime = gattpacket.time
		}
	}
	lcm.BtState = MSSubscribed
	return response, nil
}

// Subscribe negotiates the data wakeup interval with the device itself.
// currently only allows for 5-minute interval signaling
func (lcm *ConnectedMiao) Subscribe() error {
	var err error
	if err = lcm.client.Subscribe(lcm.nrfXmitChar, false, lcm.gattDataCallback); err != nil {
		log.Fatalf("XMIT subscribe failed: %s", err)
	}
	lcm.BtState = MSSubscribed
	// only know the one
	lcm.emitInterval = time.Duration(5 * time.Minute)
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

// AcceptNewSensor notifies the device to start reading the attached sensor,
// which has not yet been read by this device (it's like pairing)
func (lcm *ConnectedMiao) AcceptNewSensor() error {
	return lcm.client.WriteCharacteristic(lcm.nrfRecvChar, []byte{0xd3, 0xd1}, false)
}

// PollResponse assures that a subscription is active and returns one reading
func (lcm *ConnectedMiao) PollResponse() (*MiaoResponsePacket, error) {
	if lcm.BtState != MSSubscribed {
		err := lcm.Subscribe()
		if err != nil {
			return nil, err
		}
	}
	return lcm.MiaoResponse()
}

// MiaoLibreStatus is a helper function to return just the miaomiao's state
// i.e. new sensor, no sensor, or readings-ready
func (lcm *ConnectedMiao) MiaoLibreStatus() (MiaoDeviceState, error) {
	mp, err := lcm.PollResponse()
	if err != nil {
		return MPDeclared, err
	}
	lcm.DevState = mp.Type
	return mp.Type, err
}

// // CreateMiaoMiaoPacket deserializes a recieved packet from miaomiao
// func CreateMiaoMiaoPacket(data [363]byte) MiaoMiaoPacket {
// 	var (
// 		pktLength         uint16
// 		serialNumber      string
// 		firmwareVersion   uint16
// 		hardwareVersion   uint16
// 		batteryPercentage uint8
// 	)
// 	if data[0] != 0x28 {
// 		log.Printf("start of packet missing")
// 	}
// 	if data[362] != 0x29 {
// 		log.Printf("end of packet missing")
// 	}
// 	pktLength = binary.BigEndian.Uint16(data[1:3])
// 	serialNumber, _ = BinarySerialToString(data[5:11])
// 	firmwareVersion = binary.BigEndian.Uint16(data[14:16])
// 	hardwareVersion = binary.BigEndian.Uint16(data[16:18])
// 	batteryPercentage = uint8(data[13])
// 	var lpData [344]byte
// 	copy(lpData[:], data[18:362])
// 	lp := CreateLibrePacketNow(lpData, serialNumber)
//
// 	return MiaoMiaoPacket{
// 		data, pktLength, serialNumber, firmwareVersion, hardwareVersion, batteryPercentage, time.Time{}, time.Time{}, &lp}
// }

func CreateMiaoMiaoPacket(mmr *MiaoResponsePacket) MiaoMiaoPacket {
	var (
		pktLength         uint16
		serialNumber      string
		firmwareVersion   uint16
		hardwareVersion   uint16
		batteryPercentage uint8
	)
	if mmr.Data[0] != 0x28 {
		log.Printf("start of packet missing")
	}
	if mmr.Data[362] != 0x29 {
		log.Printf("end of packet missing")
	}
	pktLength = binary.BigEndian.Uint16(mmr.Data[1:3])
	serialNumber, _ = BinarySerialToString(mmr.Data[5:11])
	firmwareVersion = binary.BigEndian.Uint16(mmr.Data[14:16])
	hardwareVersion = binary.BigEndian.Uint16(mmr.Data[16:18])
	batteryPercentage = uint8(mmr.Data[13])
	var lpData [344]byte
	copy(lpData[:], mmr.Data[18:362])
	lp := CreateLibrePacketNow(lpData, serialNumber)

	return MiaoMiaoPacket{
		mmr.Data, pktLength, serialNumber, firmwareVersion, hardwareVersion, batteryPercentage, mmr.StartTime, mmr.EndTime, &lp}
}

// Print just gives you the deets of a miaomiao packet reading
func (mmp MiaoMiaoPacket) Print() {
	fmt.Printf("MiaoMiaoPacket\n")
	fmt.Printf("  StartTime: %v\n", mmp.StartTime)
	fmt.Printf("  EndTime: %v\n", mmp.EndTime)
	fmt.Printf("  PktLength: %v\n", mmp.PktLength)
	fmt.Printf("  SerialNumber: %v\n", mmp.SerialNumber)
	fmt.Printf("  FirmwareVersion: %v\n", mmp.FimrwareVersion)
	fmt.Printf("  HardwareVersion: %v\n", mmp.HardwareVersion)
	fmt.Printf("  BatteryPercentage: %v\n", mmp.BatteryPercentage)
}

// ToJSON serializes a MiaoMiaoPacket for transfer to a deserializer somewhere else
func (mmp MiaoMiaoPacket) ToJSON() ([]byte, error) {
	return json.Marshal(mmp)
}

func (lcm *ConnectedMiao) ReadSensor() (*MiaoMiaoPacket, error) {
	mp, err := lcm.PollResponse()
	if err != nil {
		return nil, err
	}
	if mp.Type == MPLibre {
		reading := CreateMiaoMiaoPacket(mp)
		return &reading, err
	} else {
		return nil, fmt.Errorf("did not recieve sensor response")
	}
}

// ReadingEmitter returns a channel that is hooked into a goroutine that
// blocks on BLE information transfer, and returns deserialized miaomiao packets
func (lcm *ConnectedMiao) ReadingEmitter() chan MiaoMiaoPacket {
	emitter := make(chan MiaoMiaoPacket)
	go func() {
		var mr *MiaoResponsePacket
		var err error
		for {
			if lcm.NextEmit.IsZero() {
				// log.Printf("RE First Emit")
			} else {
				// log.Printf("RE NextEmit: %v", lcm.NextEmit)
			}
			mr, err = lcm.PollResponse()
			if err != nil {
				close(emitter)
			}
			lcm.NextEmit = lcm.LastEmit.Add(lcm.emitInterval)
			// log.Printf("RE LastEmit: %v", lcm.LastEmit)
			if mr.Type == MPLibre {
				emitter <- CreateMiaoMiaoPacket(mr)
			}
			// sleep until next emit?
		}
	}()
	return emitter
}
