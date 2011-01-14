package main

import (
	"bytes"
	"fmt"
	"os"
	"bufio"
	"strings"
)

import (
	"go9p.googlecode.com/hg/p"
	"go9p.googlecode.com/hg/p/srv"
	irc "github.com/soul9/go-irc-chans"
)

type ChanCtl struct {
	srv.File
	status *bytes.Buffer
	net    *irc.Network
	ircch  string
}

type ChanLog struct {
	srv.File
	fname string
}

func (l *ChanLog) Read(fid *srv.FFid, buf []byte, offset uint64) (int, *p.Error) {
	f, err := os.Open(l.fname, os.O_RDONLY|os.O_CREATE, 0660)
	if err != nil {
		fmt.Println("Can't open logfile for reading")
		return 0, &p.Error{"Can't open logfile for reading", 0}
	}
	defer f.Close()
	count, err := f.ReadAt(buf, int64(offset))
	if err != nil && err != os.EOF {
		fmt.Println(fmt.Sprintf("Couldn't read logfile: %s", err.String()))
		return 0, &p.Error{fmt.Sprintf("Couldn't read logfile: %s", err.String()), 0}
	}
	return count, nil
}

func (ctl *ChanLog) Write(fid *srv.FFid, data []byte, offset uint64) (int, *p.Error) {
	return 0, &p.Error{"Read-only file", 0}
}

func (ctl *ChanCtl) Read(fid *srv.FFid, buf []byte, offset uint64) (int, *p.Error) {
	b := ctl.status.Bytes()[offset:]
	copy(buf, b)
	l := len(b)
	if len(buf) < l {
		l = len(buf)
	}
	return l, nil
}

func (ctl *ChanCtl) Write(fid *srv.FFid, data []byte, offset uint64) (int, *p.Error) {
	if offset > 0 {
		return 0, &p.Error{"Long writes not supported", 0}
	}
	go func() {
		lines := strings.Split(string(data), "\n", -1)
		for _, line := range lines {
			words := strings.Fields(line)
			if len(words) == 0 {
				return
			}
			fmt.Fprintf(ctl.status, ">> %v\n", words)
			switch words[0] {
			case "msg":
				if len(words) < 2 {
					fmt.Fprintf(ctl.status, "<< Not enough params %v\n", words)
					return
				}

				if err := ctl.net.Privmsg([]string{ctl.ircch}, strings.Join(words[1:], " ")); err != nil {
					fmt.Fprintf(ctl.status, "<< %v: %s\n", words, err.String())
				} else {
					fmt.Fprintf(ctl.status, "<< ok %v\n", words)
				}
				return
			}
		}
	}()
	return len(data), nil
}

func chanName(name string) string {
	c0 := name[0]
	if c0 != '#' && c0 != '&' && c0 != '+' && c0 != '!' {
		name = "#" + name
	}
	return name
}

func join(ctl *NetCtl, words []string) {
	if len(words) < 2 || len(words) > 3 {
		fmt.Fprintf(ctl.status, "<< syntax: join <network> <channel> [optional-key]\n")
		return
	}
	var key string
	if len(words) == 4 {
		key = words[2]
	}
	name := words[1]
	if _, err := ctl.net.Whois([]string{name}, ""); err != nil {
		name = chanName(name)
		if err := ctl.net.Join([]string{name}, []string{key}); err != nil { //TODO: join multiple chans, users?
			fmt.Fprintf(ctl.status, "<< couldn't join %s: %s\n", name, err.String())
			return
		}
	}

	f := new(srv.File)
	if err := f.Add(ctl.parent, name, user, nil, p.DMDIR|0771, nil); err != nil {
		fmt.Fprintf(ctl.status, "<< %v\n", err)
		return
	}

	c := new(ChanCtl)
	c.net = ctl.net
	c.status = new(bytes.Buffer)
	c.ircch = name
	if err := c.Add(f, "ctl", user, nil, 0660, c); err != nil {
		fmt.Fprintf(ctl.status, "<< %v\n", err)
		return
	}

	l := new(ChanLog)
	l.fname = *logdir + "/" + ctl.netPretty + "/" + name + ".log"
	if err := l.Add(f, "chanlog", user, nil, 0440, l); err != nil {
		fmt.Fprintf(ctl.status, "<< Couldn't create log file: %v\n", err)
		return
	}

	exch := make(chan bool) //FIXME: need to have a list of chans and their logger's exchs
	go logloop(name, c, exch, l.fname)
	fmt.Fprintf(ctl.status, "<< ok %v\n", words)
}

func logloop(ircch string, ctl *ChanCtl, exch chan bool, fname string) {
	logf, err := os.Open(fname, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
	if err != nil {
		fmt.Fprintf(ctl.status, "Couldn't open file, no logging will be performed: %s\n", ircch)
		return
	}
	defer logf.Close()

	ch := make(chan *irc.IrcMessage, 10)
	err = ctl.net.Listen.RegListener("PRIVMSG", ircch+"ircfs", ch)
	if err != nil {
		fmt.Fprintf(ctl.status, "Couldn't register listener, no logging will be performed: %s\n", ircch)
		return
	}
	defer ctl.net.Listen.DelListener("PRIVMSG", ircch+"ircfs")

	buf := bufio.NewWriter(logf)
	for {
		var msg = new(irc.IrcMessage)
		select {
		case msg = <-ch:
		case <-exch:
			return
		}
		if msg.Destination() == ircch {
			_, err := buf.WriteString(fmt.Sprintf("%s\n", msg.String()))
			if err != nil {
				fmt.Fprintf(ctl.status, "Couldn't open file, no logging will be performed: %s\n", ircch)
				return
			}
			err = buf.Flush()
			if err != nil {
				fmt.Fprintf(ctl.status, "Couldn't open file, no logging will be performed: %s\n", ircch)
				return
			}
		}
	}
}
