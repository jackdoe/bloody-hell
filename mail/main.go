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

func took(t0 int64) int64 {
	return time.Now().Unix() - t0
}

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
			t0 := time.Now().Unix()
			for _, account := range config.Accounts.List {
				err := account.refresh()
				if err != nil {
					log.Println(err)
				}
			}

			log.Printf("account fetch done, took: %d seconds", took(t0))
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
