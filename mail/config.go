package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"
	"sync"
)

var GIANT = &sync.Mutex{}
var ROOT string = GetRootDir()

type Accounts struct {
	List []*Account
}

type Config struct {
	Accounts Accounts
	Logger   *log.Logger
}

var config Config

func GetRootDir() string {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return path.Join(usr.HomeDir, ".bloody-hell")
}

func (this *Config) initialize() {
	err := os.MkdirAll(ROOT, 0700)
	if err != nil {
		panic(err)
	}

	file, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(file, &config)
	if err != nil {
		panic(err)
	}
	for _, account := range config.Accounts.List {
		for _, inbox := range account.StrInboxes {
			account.Inboxes = append(account.Inboxes, NewInbox(account, inbox))
		}
	}

	f, err := os.OpenFile(path.Join(ROOT, "log.txt"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	this.Logger = log.New(f, "bloody-hell: ", log.LstdFlags)
}
