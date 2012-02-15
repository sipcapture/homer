#
# makefile
#
# 

SHELL := /bin/bash

CC=cc 

LDFLAGS=-lpcap

CFLAGS=-g -Wall -DUSE_IPV6 -DUSE_HEP2 -DUSE_CONFFILE

main: captagent

captagent: captagent.o minIni/minIni.c
	$(CC) -o $@ $^ $(LDFLAGS)

captagent.o: captagent.c captagent.h
	$(CC) -o $@ -c $< $(CFLAGS)

clean:
	rm -f captagent *.o
	
install:
	install -m 0755 captagent /usr/local/bin/

install-cfg:
	install -m 0644 captagent.ini -D /usr/local/etc/captagent/ 
	

.PHONY: install install-cfg
	
