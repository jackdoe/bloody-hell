package main

import (
	"flag"
	"fmt"
	"log"
	"runtime"
	"time"
)

func took(t0 int64) int64 {
	return time.Now().Unix() - t0
}

func main() {
	runtime.GOMAXPROCS(1)
	config.initialize()

	var refresh int
	flag.IntVar(&refresh, "refresh", 0, "refresh")
	flag.Parse()
	if refresh > 0 {
		for {
			s := work()
			config.Logger.Printf("%s, sleeping %d seconds", s, refresh)
			time.Sleep(time.Duration(refresh) * time.Second)
		}
	} else {
		fmt.Printf("%s\n", work())
	}
}

func work() string {
	t0 := time.Now().Unix()
	total := 0
	for _, account := range config.Accounts.List {
		per_acc, err := account.refresh()
		if err != nil {
			log.Fatal(err)
		}
		total += per_acc
	}
	return fmt.Sprintf("account fetch done, took: %d seconds, for %d downloaded messages", took(t0), total)
}
