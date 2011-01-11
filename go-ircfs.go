package main

import (
	"bytes"
	"flag"
	"fmt"
	"go9p.googlecode.com/hg/p"
	"go9p.googlecode.com/hg/p/srv"
	irc "github.com/soul9/go-irc-chans"
	"log"
	"os"
	"strings"
)

type Ctl struct {
	srv.File
	status   *bytes.Buffer
	networks map[string]*irc.Network
}

type Chan struct {
	srv.File
	network, name string
	contents      *bytes.Buffer
}

var addr = flag.String("addr", ":5640", "network address")
var debug = flag.Bool("d", false, "print debug messages")


func execCmd(ctl *Ctl, words []string) {
	if len(words) == 0 {
		return
	}
	fmt.Fprintf(ctl.status, ">> %v\n", words)
	switch words[0] {
	case "connect":
		connect(ctl, words)
	case "join":
		join(ctl, words)
	}
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
	n := irc.NewNetwork(server, nick, username, realname, pass, "")
	err := n.Connect()
	if err != nil {
		fmt.Fprintf(ctl.status, "<< %v\n", err)
		return
	}
	fmt.Fprintf(ctl.status, "<< ok\n")
	ctl.networks[network] = n
	ch := make(chan *irc.IrcMessage)
	n.Listen.RegListener("*", "gircfs", ch)
	go iloop(ch)
}

func iloop(ch chan *irc.IrcMessage) {
	for {
		msg := <-ch
		fmt.Printf("!!! %v\n", msg)
	}
}

func join(ctl *Ctl, words []string) {
	if len(words) < 3 || len(words) > 4 {
		fmt.Fprintf(ctl.status, "<< syntax: join <network> <channel> [optional-key]\n")
		return
	}
	var key string
	if len(words) == 4 {
		key = words[3]
	}
	net := ctl.networks[words[1]]
	if net == nil {
		fmt.Fprintf(ctl.status, "<< invalid network\n")
		return
	}
	name := chanName(words[2])
	net.Join([]string{name}, []string{key})

	c := new(Chan)
	c.network = words[1]
	c.name = name
	c.contents = new(bytes.Buffer)
	err := c.Add(root, c.network+"_"+c.name, user, nil, 0666, ctl)
	if err != nil {
		fmt.Fprintf(ctl.status, "<< %v\n", err)
		return
	}
	fmt.Fprintf(ctl.status, "<< ok\n")

}
func chanName(name string) string {
	c0 := name[0]
	if c0 != '#' && c0 != '&' && c0 != '+' && c0 != '!' {
		name = "#" + name
	}
	return name
}


func (ctl *Ctl) Write(fid *srv.FFid, data []byte, offset uint64) (int, *p.Error) {
	if offset > 0 {
		return 0, &p.Error{"Long writes not supported", 0}
	}
	go func() {
		lines := strings.Split(string(data), "\n", -1)
		for _, line := range lines {
			execCmd(ctl, strings.Fields(line))
		}
	}()
	return len(data), nil
}


func (ctl *Ctl) Read(fid *srv.FFid, buf []byte, offset uint64) (int, *p.Error) {
	b := ctl.status.Bytes()[offset:]
	copy(buf, b)
	l := len(b)
	if len(buf) < l {
		l = len(buf)
	}
	return l, nil
}

var user = p.OsUsers.Uid2User(os.Geteuid())
var root = new(srv.File)

func main() {
	var err *p.Error

	flag.Parse()

	err = root.Add(nil, "/", user, nil, p.DMDIR|0777, nil)
	if err != nil {
		goto error
	}
	var ctl = new(Ctl)
	ctl.status = new(bytes.Buffer)
	ctl.networks = map[string]*irc.Network{}
	err = ctl.Add(root, "ctl", user, nil, 0666, ctl)
	if err != nil {
		goto error
	}

	s := srv.NewFileSrv(root)
	s.Dotu = false

	if *debug {
		s.Debuglevel = 1
	}

	s.Start(s)
	err = s.StartNetListener("tcp", *addr)
	if err != nil {
		goto error
	}
	return

error:
	log.Println(fmt.Sprintf("Error: %s %d", err.Error, err.Errornum))
}
