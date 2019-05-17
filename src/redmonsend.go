package main

import (
	"bytes"
	"embredmon"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/gomodule/redigo/redis"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

//////////////////////////////////////////////////////////////////// Constants

const (
	UPDATEINTERVAL = 600
	LOOPDURATION   = 6
	green          = "\033[32m"
	red            = "\033[31m"
	yellow         = "\033[33m"
	normal         = "\033[0m"
)

//////////////////////////////////////////////////////////////////// Data struct

type SensorConfig struct {
	Name       string   `json:"name"`
	Source     string   `json:"source"`
	Key        string   `json:"key"`
	SensorType string   `json:"type"`
	Views      []string `json:"views"`
	Warning    []string `json:"warning"`
	Alert      []string `json:"alert"`
	Refresh    uint16   `json:"refresh"`
	TTL        uint16   `json:"ttl"`
}

//////////////////////////////////////////////////////////////////// Log helpers

func Debug(logline interface{}) {
	if *debug {
		log.Println("\033[35m[Debug]\033[0m -", logline)
	}
}

func Info(logline interface{}) {
	if *verbose || *debug {
		log.Println("\033[32m[Info]\033[0m  -", logline)
	}
}

func Warning(logline interface{}) {
	log.Println("\033[33m[Warn]\033[0m  -", logline)
}

func Alert(logline interface{}) {
	log.Println("\033[31m[Alert]\033[0m -", logline)
}

//////////////////////////////////////////////////////////////////// Application specific data

var (
	verbose = flag.Bool("v", false, "Verbose")
	debug   = flag.Bool("d", false, "Debug")

	configfile = flag.String("c", "", "Use a config file (default is to use the embedded config)")

	fullkey   = flag.String("k", "", "Key (alternate + metric) to consider")
	metric    = flag.String("m", "", "Metric to consider")
	alternate = flag.String("a", "", "Specify an alterate metric prefix other than the hostname")

	override = flag.String("o", "", "Specify on override value to be used instead of running the metric script/s")
	ttl      = flag.Int("t", 0, "Specify and override value for the TTL of the metric/s")

	//TODO	loopduration = flag.Int("loopduration", 0, "Override loop cicles defaults")
	//TODO	interval     = flag.Int("interval", 0, "Override loop interval")

	loop = flag.Bool("l", false, "Enter loop mode, redmonsend will became a deamon and send metrics according to configuration found on the redis server. No other option will be used.")
	send = flag.Bool("s", false, "Run just 1 metric script and update the value on the server")
)

func DownloadScript(metric string) error {

	script := "rmscript_" + metric
	script_url := embredmon.Config.Sources[0].ScriptPrefix + "/" + script

	var ferr error

	Debug("Downloading " + script_url)

	if resp, err := http.Get(script_url); err == nil {
		if resp.Status != "200 OK" {
			Warning("Cannot download the script \"" + script + "\"")
			ferr = errors.New("Failed")
		} else {
			defer resp.Body.Close()
			if _, err := os.Stat("/tmp/rmscripts"); os.IsNotExist(err) {
				os.Mkdir("/tmp/rmscripts", 0700)
			}
			if out, err := os.Create("/tmp/rmscripts/rmscript_" + metric); err != nil {
				Warning("Cannot create the script \"/tmp/rmscripts/rmscript_" + metric + "\"")
				ferr = errors.New("Failed")
			} else {
				defer out.Close()

				if _, err = io.Copy(out, resp.Body); err != nil {
					Warning("Cannot copy the script \"rmscript_" + metric + "\"")
					ferr = errors.New("Failed")
				} else {
					if err := os.Chmod("/tmp/rmscripts/rmscript_"+metric, 0700); err != nil {
						Warning("Cannot set permission on the script \"rmscript_" + metric + "\"")
						ferr = errors.New("Failed")
					}
				}
			}
		}
	} else {
		Warning("Cannot download the script \"rmscript_" + metric + "\"")
		ferr = errors.New("Failed")
	}

	return ferr
}

func collector(confkey string, killchan chan struct{}) {
	Debug("Gorouting for configuration key \"" + yellow + confkey + normal + "\" started")

	// Connecting to redis
	var redmon_endpoint string
	var redmon_key string
	var redmon_db string

	if *configfile == "" {
		// Use the embedded configuration
		redmon_endpoint = embredmon.Config.Sources[0].Endpoint
		redmon_key = embredmon.Config.Sources[0].Key
		redmon_db = embredmon.Config.Sources[0].Db
	} else {
		Alert("Config file unimplemented")
		os.Exit(2)
	}

	c, err := redis.Dial("tcp", redmon_endpoint)
	if err == nil {
		Debug("Connection to Redis established")
	} else {
		Alert(err)
		os.Exit(2)
	}
	defer c.Close()

	// Authentication

	if _, ok := c.Do("AUTH", redmon_key); ok == nil {
		Debug("Authentication to Redis succeded")
	} else {
		Alert("Authentication to Redis failed")
		os.Exit(2)
	}

	// Database selection

	if _, ok := c.Do("SELECT", redmon_db); ok == nil {
		Debug("Selected database " + redmon_db)
	} else {
		Alert("Database selection " + redmon_db + "Falied")
		os.Exit(2)
	}

	consistent := false
	var script string
	var sensor *SensorConfig
	var refreshtime int
	var datakey string
	refreshtime = 10

	if value, err := redis.String(c.Do("GET", confkey)); err != nil {
		Warning("Load sensor configuration failed for \"" + yellow + confkey + normal + "\"")
	} else {
		if !json.Valid([]byte(value)) {
			Warning("Invalid input json for \"" + yellow + confkey + normal + "\"")
		} else {
			sensor = new(SensorConfig)
			if err := json.Unmarshal([]byte(value), sensor); err != nil {
				Warning("Loaded json does not fit into sensorconfig data structure for \"" + yellow + confkey + normal + "\"")
			} else {
				datakey = sensor.Key
				Debug("The data key for the configuration key \"" + yellow + confkey + normal + "\" is \"" + yellow + datakey + normal + "\"")
				splitted := strings.Split(datakey, ":")
				metric := strings.Join(splitted[1:], ":")
				Debug("The metric for the configuration key \"" + yellow + confkey + normal + "\" is \"" + yellow + metric + normal + "\"")
				if err := DownloadScript(metric); err != nil {
					Warning("Script download failed for metric " + metric)
				} else {
					script = "/tmp/rmscripts/rmscript_" + metric
					consistent = true
					refreshtime = int(sensor.Refresh)
				}
			}
		}
	}

	if !consistent {
		Debug("The sensor \"" + yellow + confkey + normal + "\" is incosistent")
	}

collectloop:
	for {
		select {
		case <-killchan:
			fmt.Println("killed")
			break collectloop
		case <-time.After(time.Duration(refreshtime) * time.Second):
			if consistent {
				cmd := exec.Command(script)
				var out bytes.Buffer
				cmd.Stdout = &out
				if err := cmd.Run(); err == nil {
					value := out.String()
					Debug("Executed \"" + script + "\" script")

					if sensor.TTL == uint16(0) {
						Info("TTL 0 - The value will be store forever")

						if _, err := c.Do("SET", datakey, value); err == nil {
							Info("Metric correctly set for configuration key \"" + yellow + confkey + normal + "\"")
						} else {
							Alert("Metric set failed for configuration key \"" + yellow + confkey + normal + "\"")
						}

					} else {
						Info("TTL set to " + strconv.Itoa(int(sensor.TTL)))

						if _, err := c.Do("SET", datakey, value, "EX", strconv.Itoa(int(sensor.TTL))); err == nil {
							Info("Metric correctly set for configuration key \"" + yellow + confkey + normal + "\"")
						} else {
							Alert("Metric set failed for configuration key \"" + yellow + confkey + normal + "\"")
						}
					}
				}
			}
		}
	}

	Debug("Gorouting for configuration key \"" + yellow + confkey + normal + "\" ended")
}

//////////////////////////////////////////////////////////////////// Initializzation

func init() {
	flag.Parse()
}

//////////////////////////////////////////////////////////////////// Main

func main() {

	// Connecting to redis
	var redmon_endpoint string
	var redmon_key string
	var redmon_db string

	if *configfile == "" {
		// Use the embedded configuration
		redmon_endpoint = embredmon.Config.Sources[0].Endpoint
		redmon_key = embredmon.Config.Sources[0].Key
		redmon_db = embredmon.Config.Sources[0].Db
	} else {
		Alert("Config file unimplemented")
		os.Exit(2)
	}

	c, err := redis.Dial("tcp", redmon_endpoint)
	if err == nil {
		Debug("Connection to Redis established")
	} else {
		Alert(err)
		os.Exit(2)
	}
	defer c.Close()

	// Authentication

	if _, ok := c.Do("AUTH", redmon_key); ok == nil {
		Debug("Authentication to Redis succeded")
	} else {
		Alert("Authentication to Redis failed")
		os.Exit(2)
	}

	// Database selection

	if _, ok := c.Do("SELECT", redmon_db); ok == nil {
		Debug("Selected database " + redmon_db)
	} else {
		Alert("Database selection " + redmon_db + "Falied")
		os.Exit(2)
	}

	var key string

	if *fullkey != "" {
		key = *fullkey
		if *alternate != "" {
			Warning("Alternate value will be ignored because the key has been specified")
		}
		if *metric != "" {
			Warning("Metric value will be ignored because the key has been specified")
		}
	} else {
		if *alternate != "" {
			key = *alternate + ":" + *metric
		} else {
			if hostname, err := os.Hostname(); err != nil {
				Alert("Hostname Falied")
				os.Exit(2)
			} else {
				key = hostname + ":" + *metric
			}
		}
	}

	Info("Key is " + key)

	optcount := 0
	if *send {
		optcount++
	}
	if *loop {
		optcount++
	}

	if optcount != 1 {
		Alert("Wrong options, use one among -l or -s")
		os.Exit(2)
	}

	switch {
	case *send:
		Info("Single send mode selected")

		if value, err := redis.String(c.Do("GET", key+":redmon")); err != nil {
			Warning("Load metric config failed")
		} else {
			if !json.Valid([]byte(value)) {
				Warning("Invalid input json")
			} else {
				sensor := new(SensorConfig)
				if err := json.Unmarshal([]byte(value), sensor); err != nil {
					Warning("Loaded json does not fit into sensorconfig data structure")
				} else {
					var value string
					valueok := false

					if *override != "" {
						value = *override
						valueok = true
					} else {
						if err := DownloadScript(*metric); err != nil {
							Warning("Script download failed for metric " + *metric)
						} else {
							script := "/tmp/rmscripts/rmscript_" + *metric

							cmd := exec.Command(script)
							var out bytes.Buffer
							cmd.Stdout = &out
							if err := cmd.Run(); err == nil {
								value = out.String()
								valueok = true
								Debug("Executed \"" + script + "\" script")
							}
						}
					}

					if valueok {
						if *ttl != 0 {
							Info("TTL set to " + strconv.Itoa(*ttl))

							if _, err := c.Do("SET", key, value, "EX", strconv.Itoa(*ttl)); err == nil {
								Info("Metric correctly set")
							} else {
								Alert("Metric set failed")
							}

						} else if sensor.TTL == uint16(0) {
							Info("TTL 0 - The value will be store forever")

							if _, err := c.Do("SET", key, value); err == nil {
								Info("Metric correctly set")
							} else {
								Alert("Metric set failed")
							}

						} else {
							Info("TTL set to " + strconv.Itoa(int(sensor.TTL)))

							if _, err := c.Do("SET", key, value, "EX", strconv.Itoa(int(sensor.TTL))); err == nil {
								Info("Metric correctly set")
							} else {
								Alert("Metric set failed")
							}
						}
					}
				}
			}
		}
	case *loop:
		Info("Loop mode selected")
		if *override != "" {
			Warning("Override is ignored in loop mode")
		}

		for i := 0; i < LOOPDURATION; i++ {

			Debug("Loading sensors for " + key)

			confkeys, err := redis.Strings(c.Do("KEYS", key+"*:redmon*"))
			if err != nil {
				Alert("KEYS Failed")
				os.Exit(2)
			} else {
				Debug("Sensors loaded " + fmt.Sprint(confkeys))
			}

			killchans := make([]chan struct{}, len(confkeys))

			for i, confkey := range confkeys {
				Info("Loading configuration key \"" + yellow + confkey + normal + "\"")

				killchans[i] = make(chan struct{})

				go collector(confkey, killchans[i])

			}

			time.Sleep(time.Duration(UPDATEINTERVAL) * time.Second)

			for i, confkey := range confkeys {
				Debug("Sending terminate signal to configurationkey  \"" + yellow + confkey + normal + "\" goroutine")
				killchans[i] <- struct{}{}
			}

		}

	default:
		Alert("Operating mode missing")
		os.Exit(2)
	}

}
