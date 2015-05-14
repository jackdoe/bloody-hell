package main

import (
	"log"
	//	"net/http"
	//	_ "net/http/pprof"
	"runtime"
	"time"
)

func main() {
	runtime.GOMAXPROCS(1)
	config.initialize()

	log.Print("started")
	go func() {
		for {
			for _, account := range config.Accounts {
				err := account.refresh()
				if err != nil {
					GlobalStatus = err.Error()
					log.Println(err)
				}
			}
			runtime.GC()
			time.Sleep(10 * time.Second)
		}
	}()

	//	go func() {
	//		log.Println(http.ListenAndServe("localhost:6060", nil))
	//	}()
	draw()
}
