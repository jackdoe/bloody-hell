package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	//	"github.com/davecheney/profile"
	"runtime"
	"time"
)

//go:generate genqrc qcode

func main() {
	// cfg := profile.Config{
	// 	MemProfile:     true,
	// 	NoShutdownHook: false, // do not hook SIGINT
	// }
	// defer profile.Start(&cfg).Stop()

	runtime.GOMAXPROCS(1)
	config.initialize()

	log.Print("started")
	go func() {
		for {
			for _, account := range config.Accounts.List {
				err := account.refresh()
				if err != nil {
					log.Println(err)
				}
			}
			time.Sleep(10 * time.Second)
		}
	}()

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	for {
		time.Sleep(10 * time.Second)
	}
}
