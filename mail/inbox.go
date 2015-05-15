package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"runtime"
	"sync"
)

type Inbox struct {
	DB       *sql.DB
	account  *Account
	Name     string
	lock     *sync.Mutex
	incoming chan []*Message
}

func NewInbox(account *Account, inbox string) *Inbox {
	db, err := sql.Open("sqlite3", account.DatabasePath(inbox))
	if err != nil {
		log.Panic(err)
	}
	create_table_stmt := `
        PRAGMA automatic_index = OFF;
        PRAGMA recursive_triggers = OFF;
        PRAGMA cache_spill = OFF;
        PRAGMA foreign_keys = OFF;
        PRAGMA locking_mode = EXCLUSIVE;
        PRAGMA secure_delete = OFF;
        PRAGMA synchronous = NORMAL;
        PRAGMA temp_store = 1;
        PRAGMA journal_mode = WAL;
        CREATE TABLE IF NOT EXISTS uidv (id integer not null primary key,uidv integer not null);
        CREATE TABLE IF NOT EXISTS data (uid integer not null primary key, uidv integer not null, internal_stamp_utc integer not null, document blob, did_read integer not null);`

	_, err = db.Exec(create_table_stmt)
	if err != nil {
		log.Panic(err)
	}

	s := &Inbox{DB: db, account: account, Name: inbox, lock: &sync.Mutex{}, incoming: make(chan []*Message)}

	runtime.SetFinalizer(s, func(si *Inbox) {
		if si.DB != nil {
			si.DB.Close()
			close(s.incoming)
		}
	})

	go func() {
		for {
			select {
			case m, more := <-s.incoming:
				if more {
					s.store(m)
				} else {
					return
				}
			}
		}
	}()
	return s
}

func (this *Inbox) store(que []*Message) error {
	this.lock.Lock()
	defer this.lock.Unlock()

	var err error = nil

	stmt, err := this.DB.Prepare("insert into data(uid,uidv,internal_stamp_utc,document,did_read) values(?, ?, ?, ?, ?)")
	if err != nil {
		this.panic(err)
	}
	defer stmt.Close()

	for _, m := range que {
		_, err = stmt.Exec(m.UID, m.UIDV, m.InternalStampUTC, m.RAW, 0)
		if err != nil {
			this.panic(err)
		}
	}
	this.DB.Exec(`PRAGMA shrink_memory;`)
	return err
}

func (this *Inbox) count() int {
	this.lock.Lock()
	defer this.lock.Unlock()

	var i int = 0
	err := this.DB.QueryRow("SELECT count(*) FROM data").Scan(&i)
	switch {
	case err == sql.ErrNoRows:
	case err != nil:
		this.panic(err)
	}
	return i
}

func (this *Inbox) setUIDValidity(uidv uint32) {
	this.lock.Lock()
	defer this.lock.Unlock()

	var last uint32 = 0
	err := this.DB.QueryRow("SELECT uidv FROM uidv LIMIT 1").Scan(&last)
	switch {
	case err == sql.ErrNoRows:
	case err != nil:
	default:
		if last != uidv {
			// uidvalidity changed, we can no longer trust our uids, so just delete everything
			// so it can be re-downloaded
			this.log("UIDValidity changed from %d to %d, deleting everything with the old uidvalidity", last, uidv)

			_, err := this.DB.Exec(fmt.Sprintf("DELETE from DATA where uidv = %d", last))
			if err != nil {
				this.panic(err)
			}

		}
	}

	_, err = this.DB.Exec(fmt.Sprintf("INSERT OR REPLACE INTO uidv(id,uidv) VALUES(1,%d)", uidv))
	if err != nil {
		this.panic(err)
	}
}

func (this *Inbox) GetLastUid() uint32 {
	this.lock.Lock()
	defer this.lock.Unlock()

	var i uint32 = 0
	err := this.DB.QueryRow("SELECT uid FROM data ORDER BY uid DESC LIMIT 1").Scan(&i)
	switch {
	case err == sql.ErrNoRows:
	case err != nil:
		this.panic(err)
	}
	return i
}

func encode_header(document interface{}) ([]byte, error) {
	return json.Marshal(document)
}

func decode_header(input []byte, into interface{}) error {
	return json.Unmarshal(input, into)
}

func (this *Inbox) log(format string, v ...interface{}) {
	format = fmt.Sprintf("%s: %s", this.Name, format)
	log.Printf(format, v...)
}
func (this *Inbox) panic(err error) {
	this.panicf(err.Error())
}
func (this *Inbox) panicf(format string, v ...interface{}) {
	format = fmt.Sprintf("%s: %s", this.Name, format)
	log.Panicf(format, v...)
}
