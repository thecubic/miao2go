package miao2go

import (
	"encoding/binary"
	"log"
	"time"
)

// binary.LittleEndian

type LibreReading struct {
	data [6]byte
}

type LibrePacket struct {
	Data         [344]byte
	Xmit_crcs    [3]uint16
	Minutes      uint16
	TrendIndex   uint8
	Trend        [16]LibreReading
	HistoryIndex uint8
	History      [32]LibreReading
	TimeStarted  time.Time
	SensorAge    time.Duration
	CaptureTime  time.Time
}

// LibrePacket
//

func CreateLibrePacketNow(data [344]byte) LibrePacket {
	return CreateLibrePacket(data, time.Now())
}

func CreateLibrePacket(data [344]byte, captureTime time.Time) LibrePacket {
	var (
		xmit_crcs    [3]uint16
		minutes      uint16
		trend        [16]LibreReading
		trendIndex   uint8
		history      [32]LibreReading
		historyIndex uint8
		sensorAge    time.Duration
		timeStarted  time.Time
	)

	xmit_crcs[0] = binary.LittleEndian.Uint16(data[0:2])
	xmit_crcs[1] = binary.LittleEndian.Uint16(data[24:26])
	xmit_crcs[2] = binary.LittleEndian.Uint16(data[320:322])
	minutes = binary.LittleEndian.Uint16(data[335:337])
	trendIndex = uint8(data[26])
	historyIndex = uint8(data[27])
	sensorAge = time.Duration(minutes) * time.Minute
	timeStarted = captureTime.Add(sensorAge)

	log.Printf("xmit_crcs[0]: %v", xmit_crcs[0])
	log.Printf("xmit_crcs[1]: %v", xmit_crcs[1])
	log.Printf("xmit_crcs[2]: %v", xmit_crcs[2])
	log.Printf("minutes: %v", minutes)
	log.Printf("historyIndex: %v", historyIndex)
	log.Printf("trendIndex: %v", trendIndex)
	log.Printf("timeStarted: %v", timeStarted)
	log.Printf("sensorAge: %v", sensorAge)
	log.Printf("captureTime: %v", captureTime)
	// 2:24
	// 26:320
	// 322:344
	//import "github.com/howeyc/crc16"
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

// class LibrePacket:
//     """Represents a data packet directly as read from a Freestyle Libre sensor"""
//
//     @classmethod
//     def from_bytes(cls, data: bytes, timestamp: Any = None):
//         # NOTE: Libre sensors are little-endian, regardless of the endianness
//         # of the encapsulating device
//         packet = cls()
//         packet.data = data
//
//         # L0-1: CRC of 2-23 [TODO: does not work]
//         ecrc1, crc1 = crc16.at(packet.data[0:24])
//         log.debug("crc1: %x %x %s", ecrc1, crc1, ecrc1 == crc1)
//
//         # Second arena: Sensor data
//         # L24-25: CRC of 26-319  [TODO: does not work]
//         ecrc2, crc2 = crc16.at(packet.data[24:320])
//         log.debug("crc2: %x %x %s", ecrc2, crc2, ecrc2 == crc2)
//
//         # L320-321: CRC16 of 322-343  [TODO: does not work]
//         ecrc3, crc3 = crc16.at(packet.data[320:344])
//         log.debug("crc3: %x %x %s", ecrc3, crc3, ecrc3 == crc3)
//
//         packet.minutes = struct.unpack("<h", data[335:337])[0]
//         packet.sensor_start = datetime.datetime.now() - datetime.timedelta(
//             minutes=packet.minutes
//         )
//
//         packet.history = []
//
//         nhistory = 32
//         packet.history = []
//         for imem in range(nhistory):
//             ird = (packet.index_history - imem - 1) % nhistory
//             bias = 142
//             low, high = bias + ird * 6, bias + (ird + 1) * 6
//
//             entrydata = data[low:high]
//             entryle = struct.unpack("<HHH", entrydata)
//             entry = [dtry / 8.5 for dtry in entryle]
//             packet.history.append(entry)
//             if imem == 0:
//                 log.debug("H%d@%d %s" % (imem, ird, entry))
//
//         packet.trends = []
//         ntrends = 16
//         for imem in range(ntrends):
//             ird = (packet.index_trend - imem - 1) % ntrends
//             bias = 46
//             low, high = bias + ird * 6, bias + (ird + 1) * 6
//             entrydata = data[low:high]
//             entryle = struct.unpack("<HHH", entrydata)
//             entry = [dtry / 8.5 for dtry in entryle]
//             packet.trends.append(entry)
//             if imem == 0:
//                 log.debug("T%d@%d %s" % (imem, ird, entry))
//
//         return packet
//
//     def __repr__(self):
//         return "<%s ih=%d it=%d minutes=%d start='%s'>" % (
//             type(self).__name__,
//             self.index_history,
//             self.index_trend,
//             self.minutes,
//             self.sensor_start,
//         )
//
//

//         # E3-12: the Libre serial number maybe?
//         # determine sensor serial number
//         # SN 0M00031VE4H
//         # 0m0003A74MR
//         # 0M0005GV1J8
//         log.debug("E3-12: %s", hexlify(packet.rawpacket[3:13]))
//         # E13: the battery level percentage
//         packet.battery = packet.rawpacket[13]
//         # E14-15: firmware revision
//         packet.fw_version = struct.unpack(">h", packet.rawpacket[14:16])[0]
//         # E16-17: hardware revision
//         packet.hw_version = struct.unpack(">h", packet.rawpacket[16:18])[0]
//         # E18-361: the buffered Libre packet (L)
//         packet.payload = packet.rawpacket[18:363]
//         packet.librepacket = LibrePacket.from_bytes(packet.payload, timestamp)
//         # E362: an end packet character )
//         if packet.rawpacket[packet.length - 1] != cls.end_pkt:
//             raise ValueError("envelope packet does not contain end byte")
//         return packet
//
//     def __repr__(self):
//         return "<%s battery=%d fw=%x hw=%x librepacket=%s>" % (
//             type(self).__name__,
//             self.battery,
//             self.fw_version,
//             self.hw_version,
//             self.librepacket,
//         )
