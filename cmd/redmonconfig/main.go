package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	embredmon "github.com/mmirko/redmon/pkg/embredmon"
	redmon "github.com/mmirko/redmon/pkg/redmon"

	"log"
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"
)

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

	fullkey   = flag.String("k", "", "Key (alternate + metric) to consider")
	metric    = flag.String("m", "", "Metric to consider")
	alternate = flag.String("a", "", "Specify an alterate metric prefix other than the hostname")

	save   = flag.Bool("s", false, "Save configurations as json")
	load   = flag.Bool("l", false, "Load configurations as josn")
	remove = flag.Bool("r", false, "Remove a given metric")

	ttl = flag.Int("t", 0, "Set the TTL for the metric config")

	list = flag.Bool("f", false, "Full list all server metrics")
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

	db, err := strconv.Atoi(redmon_db)
	if err != nil {
		Alert("Invalid Redis database")
		os.Exit(2)
	}

	ctx := context.Background()
	c := redis.NewClient(&redis.Options{
		Addr:     redmon_endpoint,
		Password: redmon_key,
		DB:       db,
	})
	if err := c.Ping(ctx).Err(); err != nil {
		Alert(err)
		os.Exit(2)
	}
	defer c.Close()
	Debug("Connection to Redis established")
	if redmon_key != "" {
		Debug("Authentication to Redis succeded")
	}
	Debug("Selected database " + redmon_db)

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
			if *metric != "" {
				key = *alternate + ":" + *metric
			} else {
				Alert("Missing metric")
				os.Exit(2)
			}
		} else {
			if *metric != "" {
				if hostname, err := os.Hostname(); err != nil {
					Alert("Hostname Falied")
					os.Exit(2)
				} else {
					key = hostname + ":" + *metric
				}
			} else {
				key = ""
			}
		}
	}

	Info("Key is " + key)

	switch {
	case *list:
		keys, err := c.Keys(ctx, key+"*:redmon*").Result()
		if err != nil {
			Alert("KEYS Falied")
			os.Exit(2)
		}
		for _, key := range keys {
			fmt.Println(key)
		}
	case *save:
		if key == "" {
			Alert("Missing key")
			os.Exit(2)
		}
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
			sensor := new(redmon.SensorConfig)
			if err := json.Unmarshal([]byte(configdata), sensor); err == nil {
				value, _ := json.Marshal(sensor)
				if err := c.Set(ctx, key+":redmon", string(value), 0).Err(); err != nil {
					Alert("SET key failed")
					os.Exit(2)
				} else {
					if *ttl != 0 {
						if err := c.Expire(ctx, key+":redmon", time.Duration(*ttl)*time.Second).Err(); err != nil {
							Alert("EXPIRE key failed")
							os.Exit(2)
						}
					}
				}
			} else {
				Alert("Loaded josn does not fit into sensorconfig data structure")
				os.Exit(2)
			}

		}
	case *load:
		if key == "" {
			Alert("Missing key")
			os.Exit(2)
		}
		if value, err := c.Get(ctx, key+":redmon").Result(); err == nil {
			if !json.Valid([]byte(value)) {
				Alert("Invalid input json")
				os.Exit(2)
			} else {
				sensor := new(redmon.SensorConfig)
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
		if key == "" {
			Alert("Missing key")
			os.Exit(2)
		}
		if value, err := c.Get(ctx, key+":redmon").Result(); err == nil {
			if !json.Valid([]byte(value)) {
				Alert("Invalid input json")
				os.Exit(2)
			} else {
				sensor := new(redmon.SensorConfig)
				if err := json.Unmarshal([]byte(value), sensor); err == nil {
					if err := c.Del(ctx, key+":redmon").Err(); err == nil {
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
