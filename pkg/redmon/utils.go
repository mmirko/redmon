package redmon

import "log"

func (c *RedmonConfig) Debug(logline interface{}) {
	if c != nil && c.EmitDebug {
		log.Println("\033[35m[Debug]\033[0m -", logline)
	}
}

func (c *RedmonConfig) Info(logline interface{}) {
	if c != nil && (c.Verbose || c.EmitDebug) {
		log.Println("\033[32m[Info]\033[0m  -", logline)
	}
}

func (c *RedmonConfig) Warning(logline interface{}) {
	log.Println("\033[33m[Warn]\033[0m  -", logline)
}

func (c *RedmonConfig) Alert(logline interface{}) {
	log.Println("\033[31m[Alert]\033[0m -", logline)
}
