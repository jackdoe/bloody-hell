package main

import (
	"flag"
	"log"
	"runtime"
	"time"
)

//go:generate genqrc qcode

func took(t0 int64) int64 {
	return time.Now().Unix() - t0
}

func main() {
	runtime.GOMAXPROCS(1)
	config.initialize()

	log.Print("started")

	var refresh int
	flag.IntVar(&refresh, "refresh", 0, "refresh")
	flag.Parse()
	if refresh > 0 {
		for {
			work()
			log.Printf("sleeping %d seconds", refresh)
			time.Sleep(time.Duration(refresh) * time.Second)
		}
	} else {
		work()
	}
}

func work() {
	t0 := time.Now().Unix()

	for _, account := range config.Accounts.List {
		err := account.refresh()
		if err != nil {
			log.Println(err)
		}
	}

	log.Printf("account fetch done, took: %d seconds", took(t0))
}
