package main

import (
	"fmt"
	"github.com/nsf/termbox-go"
	"io/ioutil"
	"log"
	//	"runtime"
	"strings"
	//	"sync"
	"time"
)

var tiles []*Tile = []*Tile{}
var GlobalStatus string = ""

var inboxStateChanged chan *Inbox = make(chan *Inbox)

type TextArea struct {
	text         string
	x            int
	y            int
	width        int
	height       int
	selectedLine int
	offsetLine   int
	wrap         bool
}

func NewTextArea(text string, x, y, width, height int, wrap bool) *TextArea {
	return &TextArea{text: text, x: x, y: y, width: width, height: height, selectedLine: 10000000, wrap: wrap}
}

func (this *TextArea) render(fg, bg termbox.Attribute) {
	screenW, screenH := termbox.Size()
	x := this.x
	width := this.width
	if width <= 0 {
		width = screenW
	}
	height := this.height
	if height <= 0 {
		height = screenH - this.y
	}
	endX := x + width
	line := 0

	runes := []rune(this.text)
	last := len(runes)
	for i := 0; i < last; i++ {
		r := runes[i]
		nl := false
		if r == '\n' || (i <= last-1 && r == '\r' && runes[i+1] == '\n') {
			line++
			x = this.x
			nl = true
			if r == '\r' && runes[i+1] == '\n' {
				r = '\n'
				i++
			}
		}

		if line < this.offsetLine {
			continue
		}
		if line > height {
			break
		}

		if x >= endX {
			if this.wrap {
				line++
				x = this.x
			} else {
				continue
			}
		}

		var ffg termbox.Attribute
		if line == this.selectedLine {
			ffg = fg | termbox.AttrBold
		} else {
			ffg = fg
		}

		if !nl {
			termbox.SetCell(x, this.y+line, r, ffg, bg)
			x++
		}
	}
}

type MessageList struct {
	inbox           *Inbox
	filledPageSize  int
	totalCount      int
	cursor          int
	perPage         int
	statusLine      *TextArea
	listArea        *TextArea
	messageArea     *TextArea
	initialized     bool
	selectedMessage *Message
}

func NewMessageList(inbox *Inbox, offsetY, perPage int) *MessageList {
	return &MessageList{
		inbox:          inbox,
		cursor:         0,
		filledPageSize: 0,
		totalCount:     0,
		statusLine:     NewTextArea("", 0, offsetY, -1, 1, false),
		listArea:       NewTextArea("", 0, offsetY+1, -1, perPage, false),
		messageArea:    NewTextArea("", 0, offsetY+1+perPage+1, -1, -1, true),
		perPage:        perPage,
		initialized:    false,
	}
}
func (this *MessageList) maxPages() int {
	return this.totalCount / this.perPage
}

func (this *MessageList) currentPage() int {
	return (this.cursor / this.perPage)
}

func (this *MessageList) inPageCursor() int {
	c := this.cursor % this.perPage
	if c >= this.filledPageSize {
		c = this.filledPageSize - 1
	}
	return c
}

func (this *MessageList) lineOffset() int {
	return (this.cursor / this.perPage) * this.perPage
}

func (this *MessageList) recalculateCountAndCursorOffset() {
	oldCount := this.totalCount
	this.totalCount = this.getTotalCount()
	diff := this.totalCount - oldCount
	if oldCount > 0 {
		this.cursor += diff
	}
}

func (this *MessageList) up() {
	if this.cursor > 0 {
		this.initialized = false
		this.cursor--
	}
}

func (this *MessageList) down() {
	if this.cursor < this.totalCount {
		this.initialized = false
		this.cursor++
	}
}

func (this *MessageList) redraw() {
	if !this.initialized {
		this.fill()
	}

	this.statusLine.render(termbox.ColorYellow, termbox.ColorDefault)
	this.listArea.render(termbox.ColorDefault, termbox.ColorDefault)
	this.messageArea.render(termbox.ColorDefault, termbox.ColorDefault)
}

func (this *MessageList) getTotalCount() int {
	return this.inbox.count()
}

func (this *MessageList) fill() {
	this.recalculateCountAndCursorOffset()
	offset := this.lineOffset()

	currentSubset := this.inbox.fetchBodyless(this.perPage, offset)
	this.filledPageSize = len(currentSubset)
	text := []string{}

	for i, m := range currentSubset {
		subj := m.RAW.Header.Get("Subject")
		if len(subj) == 0 {
			subj = "--no-subject--"
		}
		t, err := m.RAW.Header.Date()
		if err != nil {
			t = time.Unix(0, 0)
		}
		if i == this.inPageCursor() {
			this.listArea.selectedLine = i
		}
		t = t.Local()
		text = append(text, fmt.Sprintf("%4d/%02d/%02d %2d:%2d\t%20s\t%s", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), m.RAW.Header.Get("From"), subj))
	}

	this.listArea.text = strings.Join(text, "\n")
	this.statusLine.text = fmt.Sprintf("total messages: %d, inPageCursor: [%d]%d, currentPage: %d/%d [ %s ]", this.totalCount,
		this.cursor,
		this.inPageCursor(),
		this.currentPage(),
		this.maxPages(),
		GlobalStatus)

	if this.inPageCursor() >= 0 {
		this.selectedMessage = &currentSubset[this.inPageCursor()]
		this.inbox.fillMessageBody(this.selectedMessage)

		if this.selectedMessage.MIMEBody == nil {
			if this.selectedMessage.RAW.Body != nil {
				b, _ := ioutil.ReadAll(this.selectedMessage.RAW.Body)
				this.messageArea.text = fmt.Sprintf("ERROR: mime is null body: %s", string(b))
			} else {
				this.messageArea.text = fmt.Sprintf("ERROR: body is null")
			}
		} else {
			this.messageArea.text = fmt.Sprintf("%s", this.selectedMessage.MIMEBody.Text)
		}

	} else {
		this.messageArea.text = ""
	}

	this.initialized = true
}

type Tile struct {
	inbox         *Inbox
	selected      bool
	idx           int
	titleTextArea *TextArea
	messageList   *MessageList
}

func NewTile(inbox *Inbox, idx int) *Tile {
	tWidth := 30
	titleStartX := idx * tWidth
	t := &Tile{
		inbox:         inbox,
		idx:           idx,
		titleTextArea: NewTextArea(fmt.Sprintf("%s:%s", inbox.account.Label, inbox.name), titleStartX, 0, tWidth-1, 1, false),
		messageList:   NewMessageList(inbox, 1, 10),
	}

	return t
}
func (this *Tile) keyEvent(key termbox.Key) {
	switch key {
	case termbox.KeyCtrlP:
		this.messageList.up()
	case termbox.KeyCtrlN:
		this.messageList.down()
	case termbox.KeyTab:
		//		this.messageList.tab()
	}
}

func (this *Tile) redraw() {
	if this.selected {
		this.titleTextArea.render(termbox.ColorRed, termbox.ColorDefault)
	} else {
		this.titleTextArea.render(termbox.ColorBlue, termbox.ColorDefault)
	}
	if this.selected {
		this.messageList.redraw()
	}
}

func createTiles() {
	i := 0
	for _, account := range config.Accounts {
		for _, inbox := range account.Inboxes {
			tiles = append(tiles, NewTile(inbox, i))
			i++
		}
	}
	if len(tiles) > 0 {
		tiles[0].selected = true
	}
}
func currentSelectedTile() *Tile {
	for _, t := range tiles {
		if t.selected {
			return t
		}
	}
	panic("nothing is selected")
}
func draw() {
	err := termbox.Init()
	if err != nil {
		panic(err)
	}

	defer termbox.Close()

	termbox.SetInputMode(termbox.InputAlt | termbox.InputEsc | termbox.InputMouse)
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	createTiles()
	redraw()
	go func() {
		for {
			inbox := <-inboxStateChanged
		M:
			for _, t := range tiles {
				if t.selected && t.inbox == inbox {
					t.messageList.initialized = false
					break M
				}
			}
			log.Println("state changed: %s", inbox.name)
			redraw()
		}
	}()
	redraw()

	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			if ev.Ch == 'q' {
				return
			}
			t := currentSelectedTile()

			t.keyEvent(ev.Key)
		case termbox.EventMouse:

		case termbox.EventInterrupt:
			return
		case termbox.EventResize:

		case termbox.EventError:
			panic(ev.Err)
		}
		redraw()
	}
}

func redraw() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	for _, t := range tiles {
		t.redraw()
	}
	termbox.Flush()
}

func tb_panic(v ...interface{}) {
	termbox.Close()
	log.Panic(v)
}
