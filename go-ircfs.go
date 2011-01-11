package main

import (
	"bytes"
	"flag"
	"fmt"
	"go9p.googlecode.com/hg/p"
	"go9p.googlecode.com/hg/p/srv"
	"log"
	"os"
	"strings"
)

type Ctl struct {
	srv.File
	status *bytes.Buffer
}

var addr = flag.String("addr", "localhost:5640", "network address")
var debug = flag.Bool("d", false, "print debug messages")
var logdir = flag.String("ld", os.Getenv("HOME")+"/.go-ircfs", "irc log directory")


func (ctl *Ctl) Write(fid *srv.FFid, data []byte, offset uint64) (int, *p.Error) {
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
			case "connect":
				connect(ctl, words)
			}
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
