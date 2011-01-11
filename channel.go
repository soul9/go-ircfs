package main

import (
	"bytes"
	"fmt"
	"os"
)

import (
	"go9p.googlecode.com/hg/p"
	"go9p.googlecode.com/hg/p/srv"
	irc "github.com/soul9/go-irc-chans"
)

type ChanCtl struct {
	srv.File
	contents *bytes.Buffer
}

type ChanLog struct {
	srv.File
	contents *bytes.Buffer
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
	name := chanName(words[1])
	ctl.net.Join([]string{name}, []string{key}) //TODO: join multiple chans, users?

	f := new(srv.File)
	if err := f.Add(ctl.Parent, name, user, nil, p.DMDIR|0771, nil); err != nil {
		fmt.Fprintf(ctl.status, "<< %v\n", err)
		return
	}
	
	c := new(ChanCtl)
	c.contents = new(bytes.Buffer)
	if err := c.Add(f, name, user, nil, 0660, c); err != nil {
		fmt.Fprintf(ctl.status, "<< %v\n", err)
		return
	}

	exch := make(chan bool) //FIXME: need to have a list of chans and their logger's exchs
	logloop(name, ctl, exch)
	fmt.Fprintf(ctl.status, "<< ok\n")
}

func logloop(ircch string, ctl *NetCtl, exch chan bool) {
	logf, err := os.Open(*logdir + "/" + ctl.net.GetNetName() + "/" + ircch + ".log", os.O_APPEND|os.O_CREATE, 0660)
	if err != nil {
		fmt.Fprintf(ctl.status, "Couldn't open file, no logging will be performed: %s", ircch)
		return
	}
	defer logf.Close()

	ch := make(chan *irc.IrcMessage, 10)
	err = ctl.net.Listen.RegListener("PRIVMSG", ircch+"ircfs", ch)
	if err != nil {
		return
	}
	defer ctl.net.Listen.DelListener("PRIVMSG", ircch+"ircfs")

	for {
		var msg = new(irc.IrcMessage)
		select {
		case msg = <-ch:
		case <-exch:
			return
		}
		if m, ok := msg.Origin()["chan"]; ok && m == ircch {
			fmt.Fprintf(logf, "%s", msg.String())
		}
	}
}