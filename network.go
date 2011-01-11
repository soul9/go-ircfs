package main

import (
	"bytes"
	"fmt"
	"strings"
)

import (
	"go9p.googlecode.com/hg/p"
	"go9p.googlecode.com/hg/p/srv"
	irc "github.com/soul9/go-irc-chans"
)

type NetCtl struct {
	srv.File
	status    *bytes.Buffer
	net       *irc.Network
	parent    *srv.File
	netPretty string
}

func (ctl *NetCtl) Read(fid *srv.FFid, buf []byte, offset uint64) (int, *p.Error) {
	b := ctl.status.Bytes()[offset:]
	copy(buf, b)
	l := len(b)
	if len(buf) < l {
		l = len(buf)
	}
	return l, nil
}

func (ctl *NetCtl) Write(fid *srv.FFid, data []byte, offset uint64) (int, *p.Error) {
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
			case "join":
				join(ctl, words)
			}
		}
	}()
	return len(data), nil
}


func connect(ctl *Ctl, words []string) {
	var network, server, nick, username, realname, pass string
	if len(words) < 4 {
		return
	}
	network = words[1]
	server = words[2]
	nick = words[3]
	if len(words) < 5 {
		username = nick
	} else {
		username = words[4]
	}
	if len(words) < 6 {
		realname = username
	} else {
		realname = words[5]
	}
	if len(words) > 6 {
		pass = words[6]
	}
	n := irc.NewNetwork(server, nick, username, realname, pass, "/dev/null")
	if err := n.Connect(); err != nil {
		fmt.Fprintf(ctl.status, "<< %v\n", err)
		return
	}
	f := new(srv.File)
	if err := f.Add(root, network, user, nil, p.DMDIR|0771, nil); err != nil {
		fmt.Fprintf(ctl.status, "<< %v\n", err)
		return
	}
	c := new(NetCtl)
	c.status = new(bytes.Buffer)
	c.net = n
	c.parent = f
	c.netPretty = network
	if err := c.Add(f, "ctl", user, nil, 0660, c); err != nil {
		fmt.Fprintf(ctl.status, "<< %v\n", err)
		return
	}
	fmt.Fprintf(ctl.status, "<< ok %v\n", words)
	go keepalive(c, n)
}

func keepalive(ctl *NetCtl, n *irc.Network) {
	ch := make(chan *irc.IrcMessage)
	n.Listen.RegListener("ERROR", "gircfs", ch)
	defer n.Listen.DelListener("ERROR", "gircfs")
	for {
		msg := <-ch
		n.Reconnect("Error...")
		fmt.Fprintf(ctl.status, "Reconnect: %s", msg.String())
	}
}
