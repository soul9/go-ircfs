include $(GOROOT)/src/Make.inc

TARG=go-ircfs
GOFILES=go-ircfs.go network.go channel.go

include $(GOROOT)/src/Make.cmd
