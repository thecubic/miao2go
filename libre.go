package miao2go

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	// "strconv"
	"time"
)

const (
	historyOffset  = 142
	historyEntries = 32
	trendOffset    = 46
	trendEntries   = 16
)

type LibreReading struct {
	Data [6]byte `json:"data"`
}

type LibrePacket struct {
	Data         [344]byte        `json:"raw_data"`
	SerialNumber string           `json:"serial"`
	XmitCrcs     [3]uint16        `json:"crcs"`
	Minutes      uint16           `json:"minutes"`
	TrendIndex   int              `json:"trend_i"`
	Trend        [16]LibreReading `json:"trends"`
	HistoryIndex int              `json:"history_i"`
	History      [32]LibreReading `json:"history"`
	TimeStarted  time.Time        `json:"started"`
	SensorAge    time.Duration    `json:"age"`
	CaptureTime  time.Time        `json:"time"`
}

const (
	decoder = "0123456789ACDEFGHJKLMNPQRTUVWXYZ"
)

func firstQuint(input byte) int {
	return int((input & 0xF8) >> 3)
}

func secondQuint(input byte) int {
	return int((input & 124) >> 2)
}

func thirdQuint(input byte) int {
	return int((input & 62) >> 1)
}

func fourthQuint(input byte) int {
	return int(input & 31)
}

func threeTwo(three byte, two byte) int {
	return int((three&7)<<2 + (two&192)>>6)
}

func twoThree(two byte, three byte) int {
	return int((two&3)<<3 + (three&224)>>5)
}

func oneFour(one byte, four byte) int {
	return int((one&1)<<4 + (four&240)>>4)
}

func fourOne(four byte, one byte) int {
	return int((four&15)<<1 + (one&128)>>7)
}

func BinarySerialToString(bserial []byte) (string, error) {
	// thanks for using such a fun format, yo
	serial := fmt.Sprintf(
		"0%c%c%c%c%c%c%c%c%c%c",
		decoder[firstQuint(bserial[5])],
		decoder[threeTwo(bserial[5], bserial[4])],
		decoder[thirdQuint(bserial[4])],
		decoder[oneFour(bserial[4], bserial[3])],
		decoder[fourOne(bserial[3], bserial[2])],
		decoder[secondQuint(bserial[2])],
		decoder[twoThree(bserial[2], bserial[1])],
		decoder[fourthQuint(bserial[1])],
		decoder[firstQuint(bserial[0])],
		decoder[threeTwo(bserial[0], 0x00)],
	)
	return serial, nil
}

func StringSerialToBinary(sserial string) error {
	return nil
}

func CreateLibrePacketNow(data [344]byte, serialNumber string) LibrePacket {
	return CreateLibrePacket(data, serialNumber, time.Now())
}

func CreateLibrePacket(data [344]byte, serialNumber string, captureTime time.Time) LibrePacket {
	var (
		xmit_crcs    [3]uint16
		minutes      uint16
		trend        [16]LibreReading
		trendIndex   int
		history      [32]LibreReading
		historyIndex int
		sensorAge    time.Duration
		timeStarted  time.Time
		thisTrend    int
		thisHistory  int
	)

	// TODO: actually figure out CRC
	xmit_crcs[0] = binary.LittleEndian.Uint16(data[0:2])
	xmit_crcs[1] = binary.LittleEndian.Uint16(data[24:26])
	xmit_crcs[2] = binary.LittleEndian.Uint16(data[320:322])
	// 2:24
	// 26:320
	// 322:344
	//import "github.com/howeyc/crc16"

	minutes = binary.LittleEndian.Uint16(data[335:337])
	trendIndex = int(data[26])
	historyIndex = int(data[27])
	sensorAge = time.Duration(5*minutes) * time.Minute
	timeStarted = captureTime.Add(-sensorAge)

	// this captures readings in order of recency, descending

	for nTrend := 0; nTrend < trendEntries; nTrend++ {
		var trendData [6]byte
		thisTrend = (trendIndex - nTrend - 1 + trendEntries) % trendEntries
		_start := trendOffset + thisTrend*6
		_end := trendOffset + (thisTrend+1)*6
		// fmt.Printf("data[%v:%v]\n", _start, _end)
		copy(trendData[:], data[_start:_end])
		trend[nTrend] = LibreReading{trendData}
	}
	for nHistory := 0; nHistory < historyEntries; nHistory++ {
		var historyData [6]byte
		thisHistory = (historyIndex - nHistory - 1 + historyEntries) % historyEntries
		_start := historyOffset + thisHistory*6
		_end := historyOffset + (thisHistory+1)*6
		// fmt.Printf("data[%v:%v]\n", _start, _end)
		copy(historyData[:], data[_start:_end])
		history[nHistory] = LibreReading{historyData}
	}

	return LibrePacket{
		data,
		serialNumber,
		xmit_crcs,
		minutes,
		trendIndex,
		trend,
		historyIndex,
		history,
		timeStarted,
		sensorAge,
		captureTime,
	}
}

// SensorStatus represents the sensor
type SensorStatus byte

const (
	// SSUnknown represents no known state
	SSUnknown SensorStatus = 0x00
	// SSNotStarted means the sensor is powered on
	SSNotStarted SensorStatus = 0x01
	// SSStarting means the sensor is in 0-12h warmup
	SSStarting SensorStatus = 0x02
	// SSReady is the normal on-duty state 12h-15d
	SSReady SensorStatus = 0x03
	// SSExpired is 15d-15d12h last reading repreated
	SSExpired SensorStatus = 0x04
	// SSShutdown is at 15d12h+ it's dead
	SSShutdown SensorStatus = 0x05
	// SSFailed is any time it's otherwise broken
	SSFailed SensorStatus = 0x06
)

func (lpkt *LibrePacket) Print() {
	fmt.Printf("LibrePacket:\n")
	fmt.Printf("  SerialNumber: %v\n", lpkt.SerialNumber)
	fmt.Printf("  Xmit_crcs[0]: %v\n", lpkt.XmitCrcs[0])
	fmt.Printf("  Xmit_crcs[1]: %v\n", lpkt.XmitCrcs[1])
	fmt.Printf("  Xmit_crcs[2]: %v\n", lpkt.XmitCrcs[2])
	fmt.Printf("  Minutes: %v\n", lpkt.Minutes)
	fmt.Printf("  HistoryIndex: %v\n", lpkt.HistoryIndex)
	for idx, reading := range lpkt.History {
		fmt.Printf("  History entry %v: %v\n", idx, reading)
	}
	fmt.Printf("  TrendIndex: %v\n", lpkt.TrendIndex)
	for idx, reading := range lpkt.Trend {
		fmt.Printf("  Trend entry %v: %v\n", idx, reading)
	}
	fmt.Printf("  TimeStarted: %v\n", lpkt.TimeStarted)
	fmt.Printf("  Minutes: %v\n", lpkt.Minutes)
	fmt.Printf("  SensorAge: %v\n", lpkt.SensorAge)
	fmt.Printf("  Days: %v\n", lpkt.SensorAge.Hours()/(24.0))
	fmt.Printf("  CaptureTime: %v\n", lpkt.CaptureTime)
}

func (lpkt *LibrePacket) ToJSON() ([]byte, error) {
	return json.Marshal(lpkt)
}
