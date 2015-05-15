package main

import (
	"fmt"
	"github.com/jhillyerd/go.enmime"
	"net/mail"
	"time"
)

type Message struct {
	RAW []byte

	MIMEBody *enmime.MIMEBody
	MSG      *mail.Message

	InternalStampUTC int64
	UID              uint32
	UIDV             uint32
	DidRead          uint32
}

func (this *Message) Subject() string {
	subj := this.MSG.Header.Get("Subject")
	if len(subj) == 0 {
		subj = "--no-subject--"
	}
	return subj
}

func (this *Message) Date() string {
	t, err := this.MSG.Header.Date()
	if err != nil {
		t = time.Unix(0, 0)
	}
	t = t.Local()
	return fmt.Sprintf("%4d/%02d/%02d %2d:%2d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute())
}

func (this *Message) From() string {
	return this.MSG.Header.Get("From")
}
