package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/mxk/go-imap/imap"
	"log"
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

func (this *Account) refresh() error {
	var (
		c   *imap.Client
		cmd *imap.Command
		rsp *imap.Response
	)

	c, err := imap.DialTLS(this.Server, nil)
	if err != nil {
		return err
	}

	if c.State() == imap.Login {
		_, err = c.Login(this.User, this.Password)
		if err != nil {
			return err
		}
	} else {
		return errors.New("should be in imap.Login state")
	}

	cmd, err = imap.Wait(c.List("", "%"))
	if err != nil {
		return err
	}

	log.Println("\nTop-level mailboxes:")
	for _, rsp = range cmd.Data {
		log.Println("|--", rsp.MailboxInfo())
	}

	for _, rsp = range c.Data {
		log.Println("Server data:", rsp)
	}
	for _, inbox := range this.Inboxes {
		c.Data = nil
		c.Select(inbox.Name, true)
		inbox.log("\nMailbox status:\n", c.Mailbox)
		uidv := c.Mailbox.UIDValidity

		inbox.setUIDValidity(uidv)

		last_uid := inbox.GetLastUid()
		set, _ := imap.NewSeqSet("")
		set.AddRange(last_uid+1, 0)
		inbox.log("(uid fetch) waiting: %s", set.String())
		cmd, err = c.UIDFetch(set, "UID")
		if err != nil {
			return err
		}
		inbox.log("done UIDFetch")

		uids := []uint32{}
		for cmd.InProgress() {
			c.Recv(-1)
			for _, rsp = range cmd.Data {
				uids = append(uids, imap.AsNumber((rsp.MessageInfo().Attrs["UID"])))
			}
			cmd.Data = nil

			for _, rsp = range c.Data {
				log.Printf("%s: Server data: %s", inbox.Name, rsp)
			}
			c.Data = nil
		}
		inbox.log("done cmd.InProgress, got %d ids", len(uids))
		if len(uids) == 0 {
			return nil
		}

		per_request := 250
		set, _ = imap.NewSeqSet("")
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
			inbox.log("(header/body fetch) waiting: %s left: %d, current: %d", set.String(), len(uids), len(chunk))
			cmd, err = c.UIDFetch(set, "RFC822", "UID")
			if err != nil {
				return err
			}
			inbox.log("done UIDFetch")
			que := []*Message{}
			for cmd.InProgress() {
				// Wait for the next response (no timeout)
				c.Recv(-1)

				for _, rsp = range cmd.Data {
					info := rsp.MessageInfo()
					bmessage := imap.AsBytes(info.Attrs["RFC822"])

					msg, err := mail.ReadMessage(bytes.NewReader(bmessage))
					if err != nil {
						inbox.log(err.Error())
					} else {
						uid := imap.AsNumber((rsp.MessageInfo().Attrs["UID"]))
						m := &Message{
							MSG:              msg,
							RAW:              bmessage,
							UID:              uid,
							UIDV:             uidv,
							DidRead:          0,
							InternalStampUTC: info.InternalDate.UTC().Unix(),
						}
						que = append(que, m)
					}
				}
				cmd.Data = nil

				for _, rsp = range c.Data {
					inbox.log("Server data: %s", rsp)
				}
				c.Data = nil
			}
			if len(que) > 0 {
				inbox.incoming <- que
			}
			if last {
				break L
			}
			inbox.log("done cmd.InProgress")
			runtime.GC()
		}
	}

	if rsp, err := cmd.Result(imap.OK); err != nil {
		if err == imap.ErrAborted {
			fmt.Println("Fetch command aborted")
		} else {
			fmt.Println("Fetch error:", rsp.Info)
		}
	}
	c.Logout(1 * time.Second)
	c.Close(true)
	return nil
}
