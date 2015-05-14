package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/jhillyerd/go.enmime"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"log"
	"net/mail"
	"runtime"
	"sync"
	"time"
)

type Inbox struct {
	DB       *sql.DB
	account  *Account
	name     string
	lock     *sync.Mutex
	incoming chan *Message
	shutting chan bool
}

func NewInbox(account *Account, inbox string) *Inbox {
	db, err := sql.Open("sqlite3", account.DatabasePath(inbox))
	if err != nil {
		tb_panic(err)
	}
	create_table_stmt := `
        PRAGMA soft_heap_limit = 10000;
        PRAGMA automatic_index = OFF;
        PRAGMA recursive_triggers = OFF;
        PRAGMA page_size = 4096;
        PRAGMA cache_size = 1;
        PRAGMA cache_spill = OFF;
        PRAGMA foreign_keys = OFF;
        PRAGMA locking_mode = EXCLUSIVE;
        PRAGMA secure_delete = OFF;
        PRAGMA synchronous = NORMAL;
        PRAGMA temp_store = MEMORY;
        PRAGMA journal_mode = WAL;
        PRAGMA wal_autocheckpoint = 16384;
        CREATE TABLE IF NOT EXISTS uidv (id integer not null primary key,uidv integer not null);
        CREATE TABLE IF NOT EXISTS data (uid integer not null primary key, uidv integer not null, internal_stamp_utc integer not null, document_body blob, document_header blob, did_read integer not null);`

	_, err = db.Exec(create_table_stmt)
	if err != nil {
		tb_panic(err)
	}

	s := &Inbox{DB: db, account: account, name: inbox, lock: &sync.Mutex{}, incoming: make(chan *Message)}

	runtime.SetFinalizer(s, func(si *Inbox) {
		if si.DB != nil {
			si.DB.Close()
			close(s.incoming)
		}
	})

	go func() {
	M:
		for {
			que := []*Message{}
		L:
			for {
				select {
				case m, more := <-s.incoming:
					if more {
						que = append(que, m)
						continue
					} else {
						break M
					}
				case <-time.After(time.Second * 5):
					break L

				}
			}
			s.store(que)
		}
	}()
	return s
}

func (this *Inbox) store(que []*Message) error {
	this.lock.Lock()
	defer this.lock.Unlock()

	var err error = nil

	tx, err := this.DB.Begin()
	if err != nil {
		tb_panic(err)
	}

	stmt, err := tx.Prepare("insert into data(uid,uidv,internal_stamp_utc,document_header,document_body,did_read) values(?, ?, ?, ?, ?, ?)")
	if err != nil {
		tb_panic(err)
	}
	for _, m := range que {
		encoded_header, err := encode_header(&m.RAW.Header)
		if err != nil {
			break
		}

		body, err := ioutil.ReadAll(m.RAW.Body)
		if err != nil {
			break
		}
		_, err = stmt.Exec(m.UID, m.UIDV, m.InternalStampUTC, encoded_header, body, 0)
		if err != nil {
			break
		}
	}
	stmt.Close()
	tx.Commit()
	inboxStateChanged <- this
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
		tb_panic(err)
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
			log.Printf("UIDValidity changed from %d to %d, deleting everything with the old uidvalidity", last, uidv)

			_, err := this.DB.Exec(fmt.Sprintf("DELETE from DATA where uidv = %d", last))
			if err != nil {
				tb_panic(err)
			}

		}
	}

	_, err = this.DB.Exec(fmt.Sprintf("INSERT OR REPLACE INTO uidv(id,uidv) VALUES(1,%d)", uidv))
	if err != nil {
		tb_panic(err)
	}
}

func (this *Inbox) fetchBodyless(limit, offset int) []Message {
	this.lock.Lock()
	defer this.lock.Unlock()

	output := []Message{}
	query := fmt.Sprintf("SELECT uid,uidv, internal_stamp_utc,document_header, did_read FROM data ORDER BY uid DESC LIMIT %d OFFSET %d", limit, offset)
	rows, err := this.DB.Query(query)
	if err != nil {
		tb_panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		m := Message{RAW: &mail.Message{}}

		encoded_header := []byte{}
		err := rows.Scan(&m.UID, &m.UIDV, &m.InternalStampUTC, &encoded_header, &m.DidRead)
		if err != nil {
			tb_panic(err)
		}

		if len(encoded_header) > 0 {
			err = decode_header(encoded_header, &m.RAW.Header)
			if err != nil {
				tb_panic(err)
			}
		}
		if err != nil {
			tb_panic(err)
		}

		output = append(output, m)
	}
	return output
}

func (this *Inbox) fillMessageBody(m *Message) {
	this.lock.Lock()
	defer this.lock.Unlock()
	var b []byte = []byte{}
	err := this.DB.QueryRow(fmt.Sprintf("SELECT document_body FROM data WHERE uid = %d", m.UID)).Scan(&b)
	switch {
	case err == sql.ErrNoRows:
	case err != nil:
		tb_panic(err)
	}

	m.RAW.Body = bytes.NewReader(b)
	if len(b) > 0 {
		m.MIMEBody, err = enmime.ParseMIMEBody(m.RAW)
	}
	if err != nil {
		m.RAW.Body = bytes.NewReader([]byte(fmt.Sprintf("ERROR: %s", err.Error())))
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
		tb_panic(err)
	}
	return i
}

func encode_header(document interface{}) ([]byte, error) {
	return json.Marshal(document)
}

func decode_header(input []byte, into interface{}) error {
	return json.Unmarshal(input, into)
}
