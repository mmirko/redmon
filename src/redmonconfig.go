package main

import (
	"bufio"
	"embredmon"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gomodule/redigo/redis"
	"log"
	"os"
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

	source = flag.String("source", "", "Choose the redmon source")

	metric    = flag.String("m", "", "Metric to consider")
	alternate = flag.String("a", "", "Specify an alterate metric prefix other than the hostname")

	save   = flag.Bool("s", false, "Save configurations as json")
	load   = flag.Bool("l", false, "Load configurations as josn")
	remove = flag.Bool("r", false, "Remove a given metric")
)

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

	listmode := false

	if *metric == "" {
		listmode = true
	}

	var key string

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

	Info("Key is " + key)

	switch {
	case listmode:
		keys, err := redis.Strings(c.Do("KEYS", key+"*:redmon*"))
		if err != nil {
			Alert("KEYS Falied")
			os.Exit(2)
		}
		for _, key := range keys {
			fmt.Println(key)
		}
	case *save:
		configdata := ""
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			configdata = configdata + line
		}
		if !json.Valid([]byte(configdata)) {
			Alert("Invalid input json")
			os.Exit(2)
		} else {
			sensor := new(SensorConfig)
			if err := json.Unmarshal([]byte(configdata), sensor); err == nil {
				value, _ := json.Marshal(sensor)
				if _, err := c.Do("SET", key+":redmon", string(value)); err != nil {
					Alert("SET key failed")
					os.Exit(2)
				}
			} else {
				Alert("Loaded josn does not fit into sensorconfig data structure")
				os.Exit(2)
			}

		}
	case *load:
		if value, err := redis.String(c.Do("GET", key+":redmon")); err == nil {
			if !json.Valid([]byte(value)) {
				Alert("Invalid input json")
				os.Exit(2)
			} else {
				sensor := new(SensorConfig)
				if err := json.Unmarshal([]byte(value), sensor); err == nil {
					value, _ := json.MarshalIndent(sensor, "", "    ")
					fmt.Print(string(value))
				} else {
					Alert("Loaded josn does not fit into sensorconfig data structure")
					os.Exit(2)
				}
			}
		} else {
			Alert("Load failed")
			os.Exit(2)
		}
	case *remove:
		if value, err := redis.String(c.Do("GET", key+":redmon")); err == nil {
			if !json.Valid([]byte(value)) {
				Alert("Invalid input json")
				os.Exit(2)
			} else {
				sensor := new(SensorConfig)
				if err := json.Unmarshal([]byte(value), sensor); err == nil {
					if _, err := c.Do("DEL", key+":redmon"); err == nil {
						Info("Key \"" + key + "\" removed succesfully")
					} else {
						Alert("Key remove failed")
						os.Exit(2)
					}
				} else {
					Alert("Loaded josn does not fit into sensorconfig data structure")
					os.Exit(2)
				}
			}
		} else {
			Alert("Remove failed")
			os.Exit(2)
		}
	default:
		Alert("Operating mode missing")
		os.Exit(2)
	}
}
