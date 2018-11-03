package main

// m2g-influx: read transciever and send measurements to an InfluxDB

import (
	"flag"
	"fmt"
	"github.com/currantlabs/ble"
	"github.com/currantlabs/ble/linux"
	influx "github.com/influxdata/influxdb/client/v2"
	"github.com/thecubic/miao2go"
	"golang.org/x/net/context"
	"log"
	"time"
)

var (
	timeout        = flag.Duration("timeout", 60*time.Second, "timeout")
	miao           = flag.String("miao", "", "address of the miaomiao")
	check          = flag.Bool("check", true, "check for NewSensor condition")
	noaccept       = flag.Bool("noaccept", false, "don't accept new sensors")
	once           = flag.Bool("once", false, "don't continue after first read")
	print          = flag.Bool("print", false, "print out packet details")
	infurl         = flag.String("inf.url", "http://localhost:8086", "influxdb address")
	infuser        = flag.String("inf.user", "", "influxdb user")
	infpass        = flag.String("inf.pass", "", "influxdb password")
	infnoverifyssl = flag.Bool("inf.noverifyssl", false, "don't verify certs / hostname")
	infprefix      = flag.String("inf.prefix", "", "influxdb reporting prefix")
	infdb          = flag.String("inf.db", "sweet", "influxdb database name")
)

// influx.HTTPConfig
// influx.NewHTTPClient(conf influx.HTTPConfig)

func pingdb(hclient influx.Client) {
	latency, infversion, err := hclient.Ping(*timeout)
	if err != nil {
		log.Fatalf("can't influx ping: %s", err)
	}
	log.Printf("took %v to reach influxdb %v", latency, infversion)
}

func finddb(hclient influx.Client, db string) bool {
	query := influx.NewQuery("SHOW DATABASES", "", "")
	dbfound := false
	if result, err := hclient.Query(query); err == nil && result.Error() == nil {
		for _, qresult := range result.Results {
			for _, series := range qresult.Series {
				for _, row := range series.Values {
					if row[0] == db {
						dbfound = true
					}
				}
			}
		}
	}
	return dbfound
}

func main() {
	flag.Parse()
	var (
		latency    time.Duration
		infversion string
		hclient    influx.Client
		err        error
		reading    *miao2go.MiaoMiaoPacket
		// infready   chan struct{}
	)
	if len(*miao) == 0 {
		log.Fatalf("must pass miao")
	}

	hcfg := influx.HTTPConfig{
		Addr:      *infurl,
		Username:  *infuser,
		Password:  *infpass,
		UserAgent: "m2g-influx",
	}
	// infready = make(chan struct{})

	go func() {
		hclient, err = influx.NewHTTPClient(hcfg)
		if err != nil {
			log.Fatalf("can't influx: %s", err)
		}

		latency, infversion, err = hclient.Ping(*timeout)
		if err != nil {
			log.Fatalf("can't influx ping: %s", err)
		}
		log.Printf("took %v to reach influxdb %v", latency, infversion)
		// close(infready)

		if !finddb(hclient, *infdb) {
			log.Fatalf("inf.db %v not present", *infdb)
		}
	}()

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
		reading, err = miao.ReadSensor()
		if err == nil {
			if *print {
				reading.Print()
				reading.LibrePacket.Print()
			}
		} else {
			log.Printf("error in read attempt: %v", err)
		}
	} else {
		emitter := miao.ReadingEmitter(!*noaccept)
		for pkt := range emitter {
			if *print {
				pkt.Print()
				pkt.LibrePacket.Print()
			}
			fmt.Printf("packet captured in %v\n", pkt.EndTime.Sub(pkt.StartTime))
			fmt.Printf("next data emission scheduled for: %v\n", miao.NextEmit)
		}
	}
	cln.CancelConnection()
}
