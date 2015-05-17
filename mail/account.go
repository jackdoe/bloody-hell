package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/mxk/go-imap/imap"
	"net/mail"
	"path"
	"runtime"
	"time"
)

type Account struct {
	User       string
	Password   string
	Server     string
	Label      string
	StrInboxes []string
	Inboxes    []*Inbox
}

func (this *Account) DatabasePath(inbox string) string {
	return path.Join(ROOT, fmt.Sprintf("%s=%s=%s", this.Server, this.User, inbox))
}

func (this *Account) MaildirPath(inbox string) string {
	return path.Join(ROOT, "Maildir", this.Label, inbox)
}
func (this *Account) count() int {
	total := 0
	for _, inbox := range this.Inboxes {
		total += inbox.count()
	}
	return total
}
func (this *Account) refresh() (int, error) {
	total := 0
	c, err := imap.DialTLS(this.Server, nil)
	if err != nil {
		return total, err
	}

	c.SetLogger(config.Logger)
	c.SetLogMask(imap.LogConn | imap.LogState | imap.LogGo)
	if c.State() == imap.Login {
		_, err = c.Login(this.User, this.Password)
		if err != nil {
			return total, err
		}
	} else {
		return total, errors.New("should be in imap.Login state")
	}
	c.Data = nil

INBOX:
	for _, inbox := range this.Inboxes {
		inbox.log("refresh started")

		uidv, uids, last_uid, err := selectInboxAndFetchNewUIDS(c, inbox)
		if err != nil {
			return total, err
		}
		inbox.log("last uid: %d, current uidv: %d", last_uid, uidv)
		if len(uids) == 0 {
			continue INBOX
		}

		per_request := 50
		set, _ := imap.NewSeqSet("")
	L:
		for {
			last := false
			chunk := uids
			if len(uids) < per_request {
				last = true
			} else {
				chunk = uids[:per_request]
				uids = uids[per_request:]
			}

			set.Clear()
			for _, u := range chunk {
				// the last uid is returned if we ask for uid greather than it, so just ignore it
				if u > last_uid {
					set.AddNum(u)
				} else {
					inbox.log("ignoring %d, it is <= last_uid(%d)", u, last_uid)
				}
			}
			if set.Empty() {
				break L
			}

			t0 := time.Now().Unix()
			inbox.log("(header+body fetch) waiting: %s left: %d, current: %d", set.String(), len(uids), len(chunk))

			cmd, err := imap.Wait(c.UIDFetch(set, "RFC822", "UID"))
			if err != nil {
				return total, err
			}
			if _, err := cmd.Result(imap.OK); err != nil {
				return total, err
			}

			que := []*Message{}
			for _, rsp := range cmd.Data {
				info := rsp.MessageInfo()
				bmessage := imap.AsBytes(info.Attrs["RFC822"])

				msg, err := mail.ReadMessage(bytes.NewReader(bmessage))
				if err != nil {
					return total, err
				}
				uid := imap.AsNumber((rsp.MessageInfo().Attrs["UID"]))
				m := &Message{
					MSG:              msg,
					RAW:              bmessage,
					UID:              uid,
					UIDV:             uidv,
					DidRead:          0,
					InternalStampUTC: info.InternalDate.UTC().Unix(),
				}
				total++
				que = append(que, m)
			}
			cmd.Data = nil
			c.Data = nil

			if len(que) > 0 {
				inbox.incoming <- que
			}
			if last {
				break L
			}

			inbox.log("(header+body fetch) done cmd.InProgress, took %d", took(t0))
			runtime.GC()
		}
	}

	c.Logout(1 * time.Second)
	c.Close(true)
	return total, nil
}

func selectInboxAndFetchNewUIDS(c *imap.Client, inbox *Inbox) (uint32, []uint32, uint32, error) {
	t0 := time.Now().Unix()
	uidv := uint32(0)
	uids := []uint32{}
	last_uid := uint32(0)
	c.Data = nil
	cmd, err := c.Select(inbox.Name, true)
	if err != nil {
		return uidv, uids, last_uid, err
	}

	uidv = c.Mailbox.UIDValidity
	inbox.setUIDValidity(uidv)

	last_uid = inbox.GetLastUid() // XXX: must be after setting UIDValidity, which is destructive

	set, _ := imap.NewSeqSet("")
	set.AddRange(last_uid+1, 0)

	cmd, err = imap.Wait(c.UIDFetch(set, "UID"))
	if err != nil {
		return uidv, uids, last_uid, err
	}

	if _, err := cmd.Result(imap.OK); err != nil {
		return uidv, uids, last_uid, err
	}

	for _, rsp := range cmd.Data {
		uids = append(uids, imap.AsNumber((rsp.MessageInfo().Attrs["UID"])))
	}
	cmd.Data = nil
	c.Data = nil
	inbox.log("(uid fetch) done cmd.InProgress, got %d ids, took %d", len(uids), took(t0))
	return uidv, uids, last_uid, err
}
