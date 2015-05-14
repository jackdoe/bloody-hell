package main

import (
	"github.com/jhillyerd/go.enmime"
	"net/mail"
)

type Message struct {
	RAW              *mail.Message
	MIMEBody         *enmime.MIMEBody
	InternalStampUTC int64
	UID              uint32
	UIDV             uint32
	DidRead          uint32
}

type IncomingMessage struct {
	Message *Message
	Inbox   *Inbox
}
