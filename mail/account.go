package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/mxk/go-imap/imap"
	"log"
	"net/mail"
	"path"
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
		c.Select(inbox.name, true)
		log.Print("\nMailbox status:\n", c.Mailbox)
		uidv := c.Mailbox.UIDValidity

		inbox.setUIDValidity(uidv)

		last_uid := inbox.GetLastUid()
		set, _ := imap.NewSeqSet("")
		set.AddRange(last_uid+1, 0)
		log.Printf("(uid fetch) waiting: %s", set.String())
		cmd, err = c.UIDFetch(set, "UID")
		if err != nil {
			return err
		}
		log.Println("DONE")

		uids := []uint32{}
		for cmd.InProgress() {
			c.Recv(-1)
			for _, rsp = range cmd.Data {
				uids = append(uids, imap.AsNumber((rsp.MessageInfo().Attrs["UID"])))
			}
			cmd.Data = nil

			for _, rsp = range c.Data {
				log.Println("Server data:", rsp)
			}
			c.Data = nil
		}
		if len(uids) == 0 {
			log.Print("no new uids")
			return nil
		}
		set, _ = imap.NewSeqSet("")
		if len(uids) > 10 {
			uids = uids[:10]
		}
		set.AddNum(uids...)
		log.Printf("(header/body fetch) waiting: %s", set.String())
		cmd, err = c.UIDFetch(set, "RFC822.HEADER", "RFC822.TEXT", "UID")
		if err != nil {
			return err
		}
		log.Println("DONE")

		for cmd.InProgress() {
			// Wait for the next response (no timeout)
			c.Recv(-1)

			for _, rsp = range cmd.Data {
				info := rsp.MessageInfo()
				header := imap.AsBytes(info.Attrs["RFC822.HEADER"])
				if msg, _ := mail.ReadMessage(bytes.NewReader(header)); msg != nil {
					msg.Body = bytes.NewReader(imap.AsBytes(info.Attrs["RFC822.TEXT"]))
					uid := imap.AsNumber((rsp.MessageInfo().Attrs["UID"]))
					m := &Message{
						RAW:              msg,
						UID:              uid,
						UIDV:             uidv,
						DidRead:          0,
						InternalStampUTC: info.InternalDate.UTC().Unix(),
					}
					inbox.incoming <- m
				}
			}
			cmd.Data = nil

			for _, rsp = range c.Data {
				log.Println("Server data:", rsp)
			}
			c.Data = nil
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
