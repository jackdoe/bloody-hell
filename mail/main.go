package main

import (
	"flag"
	"fmt"
	"log"
	"sync/atomic"
	"time"
)

func took(t0 int64) int64 {
	return time.Now().Unix() - t0
}

func main() {
	var refresh int
	flag.IntVar(&refresh, "refresh", 0, "refresh")
	flag.Parse()
	config.initialize()

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
	total_new := int32(0)
	total_after := int32(0)
	wait := make(chan bool, len(config.Accounts.List))
	for _, account := range config.Accounts.List {
		go func() {
			per_acc, err := account.refresh()
			if err != nil {
				log.Fatal(err)
			}
			atomic.AddInt32(&total_new, int32(per_acc))
			atomic.AddInt32(&total_after, int32(account.count()))
			wait <- true
		}()
	}
	for range config.Accounts.List {
		<-wait
	}
	return fmt.Sprintf("took: %d seconds, for %d downloaded messages, total messages: %d", took(t0), total_new, total_after)
}
