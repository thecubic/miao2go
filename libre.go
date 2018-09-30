package miao2go

import (
	"encoding/binary"
	"fmt"
	"time"
)

const (
	historyOffset  = 142
	historyEntries = 32
	trendOffset    = 46
	trendEntries   = 16
)

type LibreReading struct {
	data [6]byte
}

type LibrePacket struct {
	Data         [344]byte
	Xmit_crcs    [3]uint16
	Minutes      uint16
	TrendIndex   int
	Trend        [16]LibreReading
	HistoryIndex int
	History      [32]LibreReading
	TimeStarted  time.Time
	SensorAge    time.Duration
	CaptureTime  time.Time
}

func CreateLibrePacketNow(data [344]byte) LibrePacket {
	return CreateLibrePacket(data, time.Now())
}

func CreateLibrePacket(data [344]byte, captureTime time.Time) LibrePacket {
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
	// actually deciminutes
	sensorAge = time.Duration(10*minutes) * time.Minute
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

func (lpkt *LibrePacket) Print() {
	fmt.Printf("LibrePacket:\n")
	fmt.Printf("  Xmit_crcs[0]: %v\n", lpkt.Xmit_crcs[0])
	fmt.Printf("  Xmit_crcs[1]: %v\n", lpkt.Xmit_crcs[1])
	fmt.Printf("  Xmit_crcs[2]: %v\n", lpkt.Xmit_crcs[2])
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
	fmt.Printf("  SensorAge: %v\n", lpkt.SensorAge)
	fmt.Printf("  CaptureTime: %v\n", lpkt.CaptureTime)
}
