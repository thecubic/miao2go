package miao2go

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"
)

var (
	zeroTime     = time.Time{}
	zeroDuration = time.Duration(0)
)

// MiaoDeviceState represents the percieved application state of the device
type MiaoDeviceState byte

// Application states
const (
	MPDeclared  MiaoDeviceState = 0x00
	MPLibre     MiaoDeviceState = 0x28
	MPNewSensor MiaoDeviceState = 0x32
	MPNoSensor  MiaoDeviceState = 0x34
)

const encapsulatedEnd = 0x29

// MiaoResponsePacket is a captured sensor read attempt
type MiaoResponsePacket struct {
	Type         MiaoDeviceState
	Data         [363]byte
	SensorPacket *LibreResponsePacket
	StartTime    time.Time
	EndTime      time.Time
}

// LibreResponsePacket is the data from the device itself
type LibreResponsePacket struct {
	Data []byte
}

// MiaoMiaoPacket is a deserialized device reading inclusive of a LibrePacket
type MiaoMiaoPacket struct {
	Data              [363]byte    `json:"raw_data"`
	PktLength         uint16       `json:"length"`
	SerialNumber      string       `json:"serial"`
	FimrwareVersion   uint16       `json:"fwver"`
	HardwareVersion   uint16       `json:"hwver"`
	BatteryPercentage uint8        `json:"batpct"`
	StartTime         time.Time    `json:"start"`
	EndTime           time.Time    `json:"end"`
	LibrePacket       *LibrePacket `json:"libre"`
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
// be sent to other functions
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
		if gattpacketlength < 10 && response.Type == MPDeclared {
			log.Printf("got: %v", gattpacket.data)
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
		return fmt.Errorf("error in hollaback write: %v", err)
	}
	return nil
}

// AcceptNewSensor notifies the device to start reading the attached sensor,
// which has not yet been read by this device (it's like pairing)
// note: does not work
func (lcm *ConnectedMiao) AcceptNewSensor() error {
	var err error
	err = lcm.client.WriteCharacteristic(lcm.nrfRecvChar, []byte{0xd3, 0xd1}, false)
	if err != nil {
		return fmt.Errorf("error in accept sensor write: %v", err)
	}
	err = lcm.client.WriteCharacteristic(lcm.nrfRecvChar, []byte{0xd1, 0x05}, false)
	if err != nil {
		return fmt.Errorf("error in tradition write: %v", err)
	}
	err = lcm.client.WriteCharacteristic(lcm.nrfRecvChar, []byte{0xf0}, false)
	if err != nil {
		return fmt.Errorf("error in hollaback write: %v", err)
	}
	// eat two GATT responses
	<-lcm.datachan
	<-lcm.datachan
	return nil
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

// CreateMiaoMiaoPacket makes an application response packet out of a raw
// datastream packet provided
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

// ReadSensor will read a sensor packet and only a sensor packet
func (lcm *ConnectedMiao) ReadSensor() (*MiaoMiaoPacket, error) {
	mp, err := lcm.PollResponse()
	if err != nil {
		return nil, err
	}
	if mp.Type == MPLibre {
		reading := CreateMiaoMiaoPacket(mp)
		return &reading, err
	}
	return nil, fmt.Errorf("did not recieve sensor response")
}

// ReadingEmitter returns a channel that is hooked into a goroutine that
// blocks on BLE information transfer, and returns deserialized miaomiao packets
func (lcm *ConnectedMiao) ReadingEmitter(accept bool) chan MiaoMiaoPacket {
	emitter := make(chan MiaoMiaoPacket)
	go func() {
		var mr *MiaoResponsePacket
		var err error
		for {
			// if lcm.NextEmit.IsZero() {
			// 	log.Printf("RE First Emit")
			// } else {
			// 	log.Printf("RE NextEmit: %v", lcm.NextEmit)
			// }
			mr, err = lcm.PollResponse()
			if err != nil {
				close(emitter)
			}
			lcm.NextEmit = lcm.LastEmit.Add(lcm.emitInterval)
			// log.Printf("RE LastEmit: %v", lcm.LastEmit)
			switch mr.Type {
			case MPLibre:
				emitter <- CreateMiaoMiaoPacket(mr)
			case MPNewSensor:
				log.Printf("MPNewSensor")
				if accept {
					// log.Printf("accepting")
					err = lcm.AcceptNewSensor()
					if err != nil {
						log.Printf("not accepted")
					} else {
						log.Printf("accepted")
					}
				}
			}
			// sleep until next emit?
		}
	}()
	return emitter
}
