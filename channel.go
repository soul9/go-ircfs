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
	status *bytes.Buffer
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

	exch := make(chan bool) //FIXME: need to have a list of chans and their logger's exchs
	go logloop(name, c, exch, ctl.netPretty)
	fmt.Fprintf(ctl.status, "<< ok %v\n", words)
}

func logloop(ircch string, ctl *ChanCtl, exch chan bool, netPretty string) {
	logf, err := os.Open(*logdir+"/"+netPretty+"/"+ircch+".log", os.O_WRONLY|os.O_CREATE, 0660)
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
