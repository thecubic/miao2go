package miao2go

import (
	"encoding/binary"
	"fmt"
	"github.com/currantlabs/ble"
	"log"
)

var (
	nrfData = ble.MustParse("6E400001-B5A3-F393-E0A9-E50E24DCCA9E")
	nrfRecv = ble.MustParse("6E400002-B5A3-F393-E0A9-E50E24DCCA9E")
	nrfXmit = ble.MustParse("6E400003-B5A3-F393-E0A9-E50E24DCCA9E")
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
}

type MiaoResponsePacket struct {
	Type         MiaoDeviceState
	Data         [363]byte
	SensorPacket *LibreResponsePacket
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
}

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
	return &ConnectedMiao{blec, nrfDataService, nrfDataRecv, nrfDataXmit,
		miaoClientDesc, MSConnected, MPDeclared, make(chan gattResponsePacket)}, nil
}

func (lcm *ConnectedMiao) gattDataCallback(data []byte) {
	if lcm.BtState == MSSubscribed {
		log.Printf("MSSubscribed -> MSBeingNotified")
		lcm.BtState = MSBeingNotified
	}
	lcm.datachan <- gattResponsePacket{data}
}

func (lcm *ConnectedMiao) MiaoResponse() (*MiaoResponsePacket, error) {
	var response *MiaoResponsePacket
	var packetData [363]byte
	packetOffset := 0
	packetFinished := false
	response = &MiaoResponsePacket{MPDeclared, packetData, nil}
	for packetFinished == false {
		gattpacket, ok := <-lcm.datachan
		gattpacketlength := len(gattpacket.data)
		if !ok {
			return nil, fmt.Errorf("datachan hangup")
		}
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
			packetFinished = true
		}
	}
	lcm.BtState = MSSubscribed
	return response, nil
}

func (lcm *ConnectedMiao) Subscribe() error {
	var err error
	if err = lcm.client.Subscribe(lcm.nrfXmitChar, false, lcm.gattDataCallback); err != nil {
		log.Fatalf("XMIT subscribe failed: %s", err)
	}
	lcm.BtState = MSSubscribed
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

func (lcm *ConnectedMiao) AcceptNewSensor() error {
	return lcm.client.WriteCharacteristic(lcm.nrfRecvChar, []byte{0xd3, 0xd1}, false)
}

func (lcm *ConnectedMiao) PollResponse() (*MiaoResponsePacket, error) {
	if lcm.BtState != MSSubscribed {
		err := lcm.Subscribe()
		if err != nil {
			return nil, err
		}
	}
	return lcm.MiaoResponse()
}

func (lcm *ConnectedMiao) MiaoLibreStatus() (MiaoDeviceState, error) {
	mp, err := lcm.PollResponse()
	if err != nil {
		return MPDeclared, err
	}
	lcm.DevState = mp.Type
	return mp.Type, err
}

type MiaoMiaoPacket struct {
	Data              [363]byte
	PktLength         uint16
	SerialNumber      [9]byte
	FimrwareVersion   uint16
	HardwareVersion   uint16
	BatteryPercentage uint8
	LibrePacket       *LibrePacket
}

func CreateMiaoMiaoPacket(data [363]byte) MiaoMiaoPacket {
	var (
		pktLength         uint16
		serialNumber      [9]byte
		firmwareVersion   uint16
		hardwareVersion   uint16
		batteryPercentage uint8
	)
	if data[0] != 0x28 {
		log.Printf("start of packet missing")
	}
	if data[362] != 0x29 {
		log.Printf("end of packet missing")
	}
	pktLength = binary.BigEndian.Uint16(data[1:3])
	copy(serialNumber[0:9], data[3:13])
	firmwareVersion = binary.BigEndian.Uint16(data[14:16])
	hardwareVersion = binary.BigEndian.Uint16(data[16:18])
	batteryPercentage = uint8(data[13])
	var lpData [344]byte
	copy(lpData[:], data[18:362])
	lp := CreateLibrePacketNow(lpData)

	return MiaoMiaoPacket{
		data, pktLength, serialNumber, firmwareVersion, hardwareVersion, batteryPercentage, &lp}
}

func (mmp MiaoMiaoPacket) Print() {
	fmt.Printf("MiaoMiaoPacket\n")
	fmt.Printf("  PktLength: %v\n", mmp.PktLength)
	fmt.Printf("  SerialNumber: %v\n", mmp.SerialNumber)
	fmt.Printf("  FirmwareVersion: %v\n", mmp.FimrwareVersion)
	fmt.Printf("  HardwareVersion: %v\n", mmp.HardwareVersion)
	fmt.Printf("  BatteryPercentage: %v\n", mmp.BatteryPercentage)
}

func (lcm *ConnectedMiao) ReadSensor() (*MiaoMiaoPacket, error) {
	mp, err := lcm.PollResponse()
	if err != nil {
		return nil, err
	}
	if mp.Type == MPLibre {
		reading := CreateMiaoMiaoPacket(mp.Data)
		return &reading, err
	} else {
		return nil, fmt.Errorf("did not recieve sensor response")
	}
}

func (lcm *ConnectedMiao) ReadingEmitter() chan MiaoMiaoPacket {
	emitter := make(chan MiaoMiaoPacket)
	go func() {
		var mr *MiaoResponsePacket
		var err error
		for {
			mr, err = lcm.MiaoResponse()
			if err != nil {
				close(emitter)
			}
			if mr.Type == MPLibre {
				emitter <- CreateMiaoMiaoPacket(mr.Data)
			}
		}
	}()
	return emitter
}
