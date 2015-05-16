package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"io"
	"log"
	"os"
	"path"
	"runtime"
	"sync"
	"time"
)

var TMP_CUR_NEW = []string{"tmp", "cur", "new"}

type Inbox struct {
	DB       *sql.DB
	account  *Account
	Name     string
	lock     *sync.Mutex
	Path     string
	incoming chan []*Message
	counter  chan uint
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
        CREATE TABLE IF NOT EXISTS data (uid integer not null primary key, uidv integer not null, filename string not null UNIQUE, internal_stamp_utc integer not null, did_read integer not null);
`

	_, err = db.Exec(create_table_stmt)
	if err != nil {
		log.Panic(err)
	}

	s := &Inbox{DB: db, account: account, Name: inbox, lock: &sync.Mutex{}, incoming: make(chan []*Message), Path: account.MaildirPath(inbox), counter: make(chan uint)}

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
	go (func() {
		for i := uint(0); true; i++ {
			s.counter <- i
		}
	})()
	s.createMailDir()
	return s
}

func (this *Inbox) store(que []*Message) error {
	this.lock.Lock()
	defer this.lock.Unlock()

	var err error = nil

	stmt, err := this.DB.Prepare("insert into data(uid,uidv,filename,internal_stamp_utc,did_read) values(?, ?, ?, ?, ?)")
	if err != nil {
		this.panic(err)
	}
	defer stmt.Close()

	for _, m := range que {
		filename := this.writeToMaildirFile(bytes.NewReader(m.RAW))
		_, err = stmt.Exec(m.UID, m.UIDV, filename, m.InternalStampUTC, 0)
		if err != nil {
			this.panicf("uid: %d, uidv: %d, filename: %s, error: %s", m.UID, m.UIDV, filename, err.Error())
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

			rows, err := this.DB.Query(fmt.Sprintf("SELECT filename FROM data ORDER BY uidv = %d", last))
			if err != nil {
				this.panic(err)
			}

			var fn string
			for rows.Next() {
				err := rows.Scan(&fn)
				if err != nil {
					this.panic(err)
				}
				for _, subdir := range TMP_CUR_NEW {
					p := path.Join(this.Path, subdir, fn)
					this.log("attempt to remove: %s", p)
					os.Remove(p)
				}
			}

			rows.Close()
			_, err = this.DB.Exec(fmt.Sprintf("DELETE from DATA where uidv = %d", last))
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
	config.Logger.Printf(format, v...)
}

func (this *Inbox) panic(err error) {
	this.panicf(err.Error())
}

func (this *Inbox) panicf(format string, v ...interface{}) {
	format = fmt.Sprintf("%s: %s", this.Name, format)
	log.Panicf(format, v...)
}

// from https://raw.githubusercontent.com/sloonz/go-maildir/master/maildir.go
func (this *Inbox) writeToMaildirFile(data io.Reader) string {
	hostname, err := os.Hostname()
	if err != nil {
		this.panic(err)
	}

	basename := fmt.Sprintf("%v.M%vP%v_%v.%v", time.Now().Unix(), time.Now().Nanosecond()/1000, os.Getpid(), <-this.counter, hostname)
	tmpname := path.Join(this.Path, "tmp", basename)
	file, err := os.Create(tmpname)
	if err != nil {
		this.panic(err)
	}

	size, err := io.Copy(file, data)
	if err != nil {
		os.Remove(tmpname)
		this.panic(err)
	}

	newbasename := fmt.Sprintf("%v,S=%v", basename, size)
	newname := path.Join(this.Path, "new", newbasename)
	err = os.Rename(tmpname, newname)
	if err != nil {
		os.Remove(tmpname)
		this.panic(err)
	}

	return newbasename
}

func (this *Inbox) createMailDir() {
	_, err := os.Stat(this.Path)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(this.Path, 0700)
			if err != nil {
				this.panic(err)
			}
			for _, subdir := range TMP_CUR_NEW {
				err = os.Mkdir(path.Join(this.Path, subdir), 0700)
				if err != nil {
					this.panic(err)
				}
			}
		} else {
			this.panic(err)
		}
	}
}
